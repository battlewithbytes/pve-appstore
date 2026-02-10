# Dockerfiles as a Native Template Format for an LXC App Store

Research document evaluating whether Dockerfiles could serve as the native template
format for an LXC-based app store, covering conversion tools, fundamental challenges,
realistic conversion rates, architecture options, and a final recommendation.

---

## 1. State of Docker-to-LXC Conversion Tools

### 1.1 Proxmox 9.1 OCI Import (November 2025)

Proxmox VE 9.1 introduced OCI image support as a **technology preview**. Users can
pull OCI images from registries (e.g., Docker Hub) directly into Proxmox and create
LXC containers from them.

**How it works:**

- Navigate to storage > CT Templates > Pull from OCI Registry
- All image layers are squashed into a single rootfs
- Proxmox distinguishes between *system containers* (full init, shell, services) and
  *application containers* (single entrypoint process)

**Current limitations:**

- Application containers are a technology preview with no stability guarantees
- Layer squashing means no incremental updates -- you must destroy and recreate
- No registry authentication support for private images
- Console shows stdout of the init process, not an interactive shell
- No `docker exec`, `docker logs`, or compose-style orchestration
- ENTRYPOINT/CMD must be manually configured after import
- Networking is LXC-native (veth pairs), not Docker bridge/overlay

**Verdict:** Useful for running simple, single-process OCI images. Not a general-purpose
Dockerfile-to-LXC pipeline. The feature addresses "I have a Docker image, can I run it
in Proxmox?" but not "I have a Dockerfile, can I build an LXC app from it."

### 1.2 Community Scripts (tteck/Proxmox, community-scripts/ProxmoxVE)

The community-scripts project (successor to tteck's original Proxmox helper scripts)
provides 400+ bash scripts that create and provision LXC containers. Each script:

- Creates an LXC container via `pct create` with sane defaults
- Pushes a provisioning script into the container
- Runs `apt-get install`, configures systemd services, sets up users

Approximately 50+ of these scripts were originally reverse-engineered from Docker
images. The maintainers report that some conversions were straightforward (static
binaries, simple web apps) while others were painful (Frigate, Authentik, Firefly).

**Key insight:** The community-scripts project validates the approach of writing
imperative provisioning scripts rather than trying to reuse Dockerfiles directly.
Their 400+ scripts are all hand-written bash, not auto-generated from Dockerfiles.

### 1.3 docker2lxc Projects

Several small projects exist for converting Docker images to LXC rootfs tarballs:

- **diraneyya/docker2lxc** -- shell script that exports a Docker image as a
  `.tar.gz` rootfs usable as an LXC template
- **fabiofalci/export-docker** -- exports a running Docker container's filesystem
  to LXC format
- **skopeo + umoci** -- OCI toolchain that can pull and unpack images without Docker

All of these operate at the **filesystem layer** -- they extract the final rootfs from
a Docker image. None of them handle:

- Init system setup (systemd unit files, service management)
- Entrypoint/CMD translation to persistent services
- Environment variable injection via LXC config
- Volume mount semantics
- Network configuration

### 1.4 Incus / LXD OCI Support

Incus (the LXD successor) added OCI image support in version 6.3:

```bash
incus remote add docker https://registry-1.docker.io/v2 --protocol=oci --public
incus launch docker:nginx --ephemeral
```

This is more mature than Proxmox's implementation but faces the same fundamental
problem: LXD/Incus runs **system containers** that expect `/sbin/init`, while Docker
images are **application containers** that expect a single foreground process.

### 1.5 Podman Rootless + Quadlet

Podman offers an alternative angle: run OCI containers under systemd using Quadlet
(available since Podman 4.6). Quadlet converts declarative `.container` unit files
into systemd services:

```ini
# /etc/containers/systemd/myapp.container
[Container]
Image=docker.io/library/nginx:latest
PublishPort=8080:80

[Service]
Restart=always

[Install]
WantedBy=multi-user.target
```

This is interesting because it bridges the Docker and systemd worlds, but it still
requires a full Podman/OCI runtime inside the LXC container -- adding complexity and
a layer of indirection that defeats the purpose of native LXC provisioning.

---

## 2. Fundamental Challenges

### 2.1 Init Systems: Docker CMD/ENTRYPOINT vs systemd/openrc

This is the single biggest impedance mismatch.

**Docker model:**
- PID 1 is the application process (or a thin wrapper like `tini`/`dumb-init`)
- Process dies = container dies
- No service manager, no cron, no syslog

**LXC model:**
- PID 1 is an init system (systemd, openrc, or similar)
- Services are managed via unit files or init scripts
- Container is a long-running system with multiple services

**Translation problem:** A Dockerfile's `CMD ["nginx", "-g", "daemon off;"]` must
become a systemd unit file:

```ini
[Unit]
Description=nginx
After=network.target

[Service]
ExecStart=/usr/sbin/nginx -g "daemon off;"
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

This translation is theoretically automatable for simple cases, but breaks down when:

- The entrypoint is a shell script that sets up environment variables, runs migrations,
  generates configs, then exec's the main process
- Multiple processes are coordinated (e.g., supervisord, s6-overlay)
- The container expects Docker's signal handling (SIGTERM -> graceful shutdown)

### 2.2 Multi-Stage Builds

Docker multi-stage builds are a build-time optimization with no LXC equivalent:

```dockerfile
FROM golang:1.24 AS builder
COPY . .
RUN go build -o /app ./cmd/server

FROM debian:12-slim
COPY --from=builder /app /usr/local/bin/app
CMD ["/usr/local/bin/app"]
```

The `builder` stage compiles code; only the final binary is copied to the runtime
image. In an LXC context:

- There is no build stage -- the container IS the final environment
- You would need to either (a) compile on the host and push the binary in, or
  (b) install build tools in the container, build, then remove them
- Neither approach can be derived mechanically from the Dockerfile

### 2.3 Networking

| Aspect | Docker | LXC |
|--------|--------|-----|
| Default network | bridge (docker0) with NAT | veth pair on host bridge |
| Port mapping | `-p 8080:80` (iptables DNAT) | Container gets its own IP |
| DNS | Built-in DNS server | Inherits host `/etc/resolv.conf` |
| Service discovery | Container name resolution | Not built-in |
| Overlay networks | Yes (Swarm, Compose) | No equivalent |

Docker's `EXPOSE` directive is purely documentation -- it does not actually configure
networking. LXC containers get a full network stack with their own IP, so port mapping
is unnecessary but the application must bind to `0.0.0.0` (which many Dockerized apps
already do).

### 2.4 Volume Semantics

**Docker:**
- Named volumes (`docker volume create`)
- Bind mounts (`-v /host/path:/container/path`)
- tmpfs mounts
- Volume drivers (NFS, cloud storage)

**LXC:**
- Mount points in container config (`mp0: /host/path,mp=/container/path`)
- ZFS datasets, LVM volumes, or directory-based storage
- No abstract volume driver layer

A Dockerfile's `VOLUME /data` declaration creates an anonymous volume at runtime.
Translating this to LXC requires deciding on storage backend, size, and mount
configuration -- decisions that cannot be inferred from the Dockerfile alone.

### 2.5 Privilege Levels and Security

Docker containers typically run as root inside a user namespace (rootless Docker) or
as a non-root user specified by `USER` directive. LXC has a more complex model:

- **Unprivileged containers** -- UID/GID mapping via subordinate ID ranges
- **Privileged containers** -- root in container = root on host (dangerous)
- **AppArmor/seccomp profiles** -- applied at LXC config level
- **Capabilities** -- dropped/added via LXC config, not per-process

A Dockerfile's `USER nobody` has very different security implications in LXC, where
the container's init system needs to run as root (UID 0, mapped or not).

### 2.6 Package Management Assumptions

Dockerfiles assume a minimal base image and install only what is needed:

```dockerfile
FROM alpine:3.20
RUN apk add --no-cache nginx
```

LXC system containers start from a full OS template (Debian, Ubuntu, Alpine) that
includes systemd/openrc, common utilities, and package management. The package
installation commands are similar, but:

- Docker images are built with `--no-cache` / `rm -rf /var/lib/apt/lists/*` to
  minimize size -- this is unnecessary and counterproductive in LXC
- Alpine-based Docker images use musl libc, while most LXC templates use glibc
- Docker's layer caching means `RUN apt-get update` is cached; LXC has no equivalent

### 2.7 Build Context and COPY/ADD

Dockerfiles use `COPY` and `ADD` to bring files from the build context into the image.
In LXC provisioning, these files must be pushed into the running container via
`pct push` or embedded in the provisioning script. There is no build context concept.

---

## 3. Realistic Conversion Rates

Based on analysis of Docker Hub's top 100 official images and the community-scripts
project's experience converting 50+ Docker applications to LXC:

### Tier 1: Fully Automatic Conversion (~10-15%)

Apps that could be mechanically converted from Dockerfile to LXC provisioning:

- **Single static binary** (e.g., `COPY binary /usr/local/bin/ && CMD ["/usr/local/bin/binary"]`)
- **Simple apt/apk install + service** (e.g., nginx, redis, memcached)
- **No entrypoint script**, no multi-stage build, no build-time compilation
- **No Docker-specific features** (healthcheck polling, Docker secrets, compose deps)

Examples: redis, memcached, registry (distribution), traefik (binary download)

### Tier 2: Human-Assisted Conversion (~40-50%)

Apps where a Dockerfile provides useful metadata but requires human judgment:

- **Entrypoint scripts** that need rewriting as systemd ExecStartPre/ExecStart
- **Environment variables** that need mapping to config files
- **Single-stage builds** with complex `RUN` chains (can be partially extracted)
- **Known upstream install scripts** (e.g., Ollama's `curl | sh`)

The Dockerfile serves as a **recipe reference** -- a developer reads it to understand
what packages are needed, what config files to create, and what services to start, then
writes a native provisioning script.

Examples: postgres, mariadb, node apps, Python web apps, home-assistant, jellyfin

### Tier 3: Complete Rewrite Required (~35-50%)

Apps where the Dockerfile is essentially useless for LXC conversion:

- **Multi-stage builds** with compilation steps (Go, Rust, C++)
- **Complex entrypoint orchestration** (supervisord, s6-overlay, tini + shell scripts)
- **Docker-specific assumptions** (Docker socket access, container networking, sidecars)
- **Vendor-specific runtimes** (Java app servers with custom classloaders, .NET)
- **Multi-container stacks** (app + DB + cache + proxy in separate containers)

Examples: GitLab, Authentik, Nextcloud (with its many microservices), Frigate,
most CI/CD tools

### Summary Table

| Tier | % of Dockerfiles | Conversion Method | Dockerfile Usefulness |
|------|------------------|-------------------|-----------------------|
| Fully automatic | 10-15% | Parse and translate | High |
| Human-assisted | 40-50% | Parse, extract metadata, human edits | Medium (reference) |
| Complete rewrite | 35-50% | Read for understanding only | Low (inspiration only) |

---

## 4. Architecture Options

### 4.1 Current Approach: Python SDK with install.py Scripts

The current PVE App Store architecture uses a Python SDK (`appstore.BaseApp`) with
declarative manifests (`app.yml`) and imperative provisioning scripts (`install.py`).

**Example (Ollama):**

```yaml
# app.yml
lxc:
  ostemplate: debian-12
  defaults:
    cores: 4
    memory_mb: 8192
provisioning:
  script: provision/install.py
  timeout_sec: 1800
```

```python
# provision/install.py
class OllamaApp(BaseApp):
    def install(self):
        self.run_installer_script("https://ollama.ai/install.sh")
        self.write_config("/etc/systemd/system/ollama.service.d/override.conf", ...)
        self.restart_service("ollama")
```

**Strengths:**

- Full control over init system integration (systemd/openrc)
- Permission-gated operations (SDK checks allowlists before executing)
- Inputs/outputs model maps cleanly to Proxmox's TUI and web UI
- Scripts are reviewable, auditable, and testable
- SDK provides cross-distro helpers (`apt_install`, `create_service`, `create_venv`)
- Uninstall/healthcheck/configure lifecycle hooks

**Weaknesses:**

- Every new app requires writing Python code
- No reuse of existing Docker ecosystem knowledge
- Steeper contribution barrier vs "just provide a Dockerfile"

### 4.2 Hypothetical: Dockerfile as Native Format

In this model, contributors would submit Dockerfiles that the engine parses and
translates to LXC provisioning steps at deploy time.

**What the parser would need to handle:**

```
FROM debian:12           -> Select LXC OS template
RUN apt-get install ...  -> apt_install() calls
COPY file /path          -> pct push file into container
ENV KEY=VALUE            -> Write to /etc/environment or service env
EXPOSE 8080              -> Document in outputs (no actual port mapping needed)
USER appuser             -> create_user() + chown
VOLUME /data             -> Create LXC mount point
CMD ["./app"]            -> Generate systemd unit file
ENTRYPOINT ["./entry"]   -> Generate systemd ExecStart
WORKDIR /app             -> cd in provisioning script
```

**What would break:**

- `FROM` with non-Debian/Ubuntu/Alpine base images (custom images, distroless, scratch)
- `FROM ... AS builder` multi-stage builds (no equivalent)
- `RUN` commands that assume Docker's layer caching or ephemeral filesystem
- `COPY --from=builder` cross-stage copies
- Shell-form `RUN` with `&&` chains that mix build and runtime concerns
- `HEALTHCHECK` with Docker-specific retry semantics
- `.dockerignore` and build context assumptions
- `ARG` for build-time variables (no build phase in LXC)
- Docker Compose `depends_on`, `networks`, `volumes` orchestration

**Estimated coverage:** A Dockerfile parser could handle maybe 25-30% of real-world
Dockerfiles without human intervention. The remaining 70-75% would either fail to
parse or produce broken LXC containers.

### 4.3 Hybrid: Dockerfile as Import Format (Recommended)

Use Dockerfiles as an **import/scaffolding** source rather than a runtime format:

```
Dockerfile  -->  [Parser]  -->  app.yml (manifest) + install.py (scaffold)
                                     |
                            [Human review & edit]
                                     |
                              Final app package
```

The parser would extract:

- Base OS and version from `FROM` -> `lxc.ostemplate` in `app.yml`
- `RUN apt-get install` packages -> `apt_install()` calls in `install.py`
- `ENV` declarations -> inputs with defaults in `app.yml`
- `EXPOSE` ports -> outputs in `app.yml`
- `COPY` local files -> `deploy_provision_file()` calls
- `CMD`/`ENTRYPOINT` -> `create_service()` call in `install.py`
- `VOLUME` declarations -> `volumes` in `app.yml`
- `USER` directives -> `create_user()` calls

The developer then reviews, adjusts, and tests the generated scaffold before
submitting it to the catalog.

---

## 5. Recommendation

**Keep the current Python SDK approach as the native format. Add Dockerfile import
as a scaffolding tool.**

### Why Dockerfile Should NOT Be the Native Format

1. **Impedance mismatch is fundamental, not incidental.** Docker and LXC have
   different process models (single-process vs. init system), different networking
   (NAT + port mapping vs. bridged IP), and different lifecycle semantics (ephemeral
   vs. persistent). These differences cannot be papered over by a translation layer.

2. **Multi-stage builds have no LXC equivalent.** A significant and growing percentage
   of production Dockerfiles use multi-stage builds. Any Dockerfile-native approach
   would need to either forbid them (rejecting much of Docker's ecosystem) or
   implement a build pipeline that defeats the purpose of using Dockerfiles.

3. **Init system translation is a bottomless pit.** Converting `CMD`/`ENTRYPOINT` to
   systemd units works for trivial cases but breaks for entrypoint scripts that do
   runtime configuration, migration, secret injection, or process supervision. The
   community-scripts project's experience confirms this -- their most painful
   conversions were apps with complex entrypoints.

4. **The Python SDK provides security guarantees.** The permission allowlist system
   (`permissions.packages`, `permissions.paths`, `permissions.services`) enables
   security auditing of what an install script can do. Dockerfiles have no equivalent
   -- a `RUN` command can do anything. Adopting Dockerfile as native format would
   mean either abandoning security controls or building a Dockerfile linter that
   understands the full semantics of every shell command.

5. **Proxmox's OCI import validates the boundary.** Proxmox 9.1 added OCI import as a
   technology preview and immediately hit the same walls: no updates, no shell access
   in app containers, layer squashing, and limited networking. If Proxmox upstream
   considers this experimental after years of development, a third-party app store
   should not build its foundation on it.

6. **The 400+ community-scripts prove the imperative approach works.** The largest
   LXC app ecosystem in existence (tteck/community-scripts) uses hand-written
   provisioning scripts, not Dockerfile translation. This is strong evidence that
   the scripted approach is the right one for LXC.

### What Dockerfile Import Should Look Like

Implement a `pve-appstore dev import-dockerfile <path>` command that:

1. Parses the Dockerfile using a simple line-by-line parser
2. Extracts `FROM` -> OS template, `RUN apt-get` -> packages, `ENV` -> inputs,
   `EXPOSE` -> outputs, `CMD` -> service definition
3. Generates a scaffold `app.yml` and `install.py` with TODO comments for
   anything it could not automatically translate
4. Warns about multi-stage builds, complex `RUN` chains, and Docker-specific features
5. Opens the generated files in the developer mode editor for review

This gives contributors a starting point while keeping the native format clean,
auditable, and LXC-native.

### Summary

| Approach | Complexity | Coverage | Security | Maintainability |
|----------|-----------|----------|----------|-----------------|
| Python SDK (current) | Low | 100% of LXC apps | Strong (permission allowlists) | High |
| Dockerfile native | Very high | ~25-30% of Dockerfiles | Weak (arbitrary RUN) | Low |
| Dockerfile import + SDK | Medium | 100% of LXC apps + Docker scaffolding | Strong | High |

The recommended path is **Dockerfile as import, Python SDK as native format**.

---

## References

- [Proxmox VE 9.1 OCI Image Support](https://forum.proxmox.com/threads/oci-images-in-lxc-release-9-1.176273/)
- [Proxmox OCI Containers First Look (Techno Tim)](https://technotim.com/posts/proxmox-oci-images/)
- [Proxmox OCI Containers Guide (Raymii.org)](https://raymii.org/s/tutorials/Finally_run_Docker_containers_natively_in_Proxmox_9.1.html)
- [community-scripts/ProxmoxVE](https://github.com/community-scripts/ProxmoxVE)
- [Docker-to-LXC Conversion Discussion (#5929)](https://github.com/community-scripts/ProxmoxVE/discussions/5929)
- [Docker to PVE-LXC Conversion Forum Thread](https://forum.proxmox.com/threads/docker-to-pve-lxc-conversion-steps-tool.143193/)
- [diraneyya/docker2lxc](https://github.com/diraneyya/docker2lxc)
- [fabiofalci/export-docker](https://github.com/fabiofalci/export-docker)
- [Incus OCI Image Support](https://github.com/lxc/incus/issues/908)
- [Running OCI Images in Incus (Simos Blog)](https://blog.simos.info/running-oci-images-i-e-docker-directly-in-incus/)
- [Lxdocker: Convert Docker Images to LXD](https://discuss.linuxcontainers.org/t/lxdocker-convert-docker-images-to-lxd-images/14790)
- [Docker and LXC Comparison (iay.org.uk)](https://iay.org.uk/blog/2024/02/09/docker-and-lxc/)
- [LXC vs Docker: Same Kernel, Different Worlds](https://thelinuxcode.com/lxc-vs-docker-containers-same-kernel-primitives-different-operational-worlds/)
- [Podman Quadlet systemd Integration](https://blog.christophersmart.com/2021/02/20/rootless-podman-containers-under-system-accounts-managed-and-enabled-at-boot-with-systemd/)
