# Security Model

This document describes how PVE App Store runs on a Proxmox VE host with least-privilege design.

## Privilege Separation

The service runs as the unprivileged `appstore` Linux user under systemd. It never runs as root.

### Proxmox REST API (primary)

Container lifecycle operations (create, start, stop, shutdown, destroy, status) and cluster queries (next CTID, template listing) use the **Proxmox REST API** via an API token. This avoids privilege escalation entirely for these operations — the API token carries only the permissions granted to it.

### Shell operations (fallback)

Six operations have no REST API equivalent (or are restricted to `root@pam` in the API) and require `sudo` via a strict sudoers allowlist:

- `pct exec` — run commands inside a container
- `pct push` — copy files from the host into a container
- `pct set` — configure device passthrough (GPU) on a container
- `tee -a` — append raw LXC config lines (`extra_config`) to `/etc/pve/lxc/*.conf`
- `mkdir -p` — create directories on host storage pools for bind mounts
- `chown` — fix ownership on bind mount paths for unprivileged containers

The Proxmox API restricts `dev*` parameters (device passthrough) to `root@pam` only — API tokens cannot set them. To work around this, containers are created via the API without device params, and `pct set` is used post-creation to apply device passthrough on the host.

The `tee -a` command is needed because `/etc/pve/lxc/` is a FUSE-mounted cluster filesystem that requires root access. Apps with `extra_config` in their manifest (e.g. `lxc.cap.add: net_admin`) have these lines appended to the container config file after creation.

The `mkdir` command is needed because bind mount target directories often live on storage pools (ZFS datasets, LVM, etc.) owned by root. The service process cannot write there due to `ProtectSystem=strict` and filesystem ownership. The file browser's "create directory" feature uses this to prepare host paths before container creation.

The `chown` command is needed for unprivileged containers with bind mounts. In unprivileged containers, UID 0 maps to host UID 100000. Bind mount host directories are typically owned by host UID 0, so the container's root cannot chown files inside them (EPERM). Before starting the container, the engine chowns bind mount paths to `100000:100000` so the container's mapped root has ownership.

These are the **only** commands permitted via `/etc/sudoers.d/pve-appstore`.

### Unprivileged service process

The `pve-appstore.service` unit applies systemd sandboxing:

| Directive | Effect |
|-----------|--------|
| `User=appstore` | Process runs as unprivileged user |
| `ProtectSystem=strict` | `/usr`, `/boot`, `/efi`, `/etc` are read-only |
| `ProtectHome=yes` | `/home`, `/root`, `/run/user` are inaccessible |
| `PrivateTmp=yes` | Service gets an isolated `/tmp` |
| `ReadWritePaths=` | Only `/var/lib/pve-appstore` and `/var/log/pve-appstore` are writable |
| `NoNewPrivileges=no` | Required to allow `sudo` for `pct exec`/`pct push`/`pct set` child processes |

All web server, API handler, catalog parsing, and general application code runs within this sandbox.

### Sudoers allowlist

Only these commands are permitted via `/etc/sudoers.d/pve-appstore`:

```
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct exec *
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct push *
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct set *
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/bin/tee -a /etc/pve/lxc/*
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/bin/mkdir -p *
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/bin/chown *
```

No other commands can be run as root by the `appstore` user.

## Mount Namespace Escape (nsenter)

### The problem

`ProtectSystem=strict` creates a read-only mount namespace for the service process. This namespace is **inherited by all child processes**, including those run via `sudo`. When `pct exec` runs as root, it still cannot write to paths like `/run/lock/lxc/` because the mount namespace makes them read-only.

### The solution

All privileged commands are wrapped with `nsenter --mount=/proc/1/ns/mnt --` which re-enters PID 1's (host init) mount namespace before executing the command:

```
sudo /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct exec 104 -- ...
```

This means:

- The **main service process stays fully sandboxed** — web server, API handlers, catalog parser, etc. all run under `ProtectSystem=strict`
- Only `pct exec`, `pct push`, `pct set`, `tee`, `mkdir`, and `chown` child processes escape to the host mount namespace
- Only the mount namespace is affected — PID, network, user, and other namespaces remain unchanged

### Why not ProtectSystem=full?

Downgrading to `ProtectSystem=full` would weaken security for the **entire service** (making `/etc` writable) just to accommodate child processes. The nsenter approach keeps `/etc` read-only for the service while allowing only allowlisted commands to see the real filesystem.

| | `ProtectSystem=full` | nsenter approach |
|---|---|---|
| Service process | `/usr` and `/boot` read-only | `/usr`, `/boot`, `/efi`, **`/etc`** all read-only |
| Child processes | All see writable `/etc`, `/run`, `/var` | Only allowlisted commands see writable host |
| Attack surface | Broader | Narrower |

## API vs Shell attack surface

| Operation | Before (shell) | After (API) |
|-----------|----------------|-------------|
| Container create/start/stop/destroy | `sudo pct ...` (11 sudoers entries) | REST API via token (0 sudoers entries) |
| CTID allocation | `sudo pvesh get /cluster/nextid` | REST API `GET /cluster/nextid` |
| Template listing | `sudo pveam list` | REST API `GET /nodes/{node}/storage/{storage}/content` |
| Container exec | `sudo pct exec` | `sudo pct exec` (no API equivalent) |
| File push | `sudo pct push` | `sudo pct push` (no API equivalent) |
| Device passthrough | `sudo pct set` | `sudo pct set` (API restricted to root@pam) |
| Host directory creation | N/A | `sudo mkdir -p` (for bind mount paths on storage pools) |
| Bind mount ownership | N/A | `sudo chown` (UID shift for unprivileged containers) |
| Extra LXC config | N/A | `sudo tee -a` (append to `/etc/pve/lxc/*.conf`) |

The API approach:
- **Reduces sudoers entries from 11 to 6**
- **Eliminates `sudo` privilege escalation** for most operations
- **Uses the API token** with scoped permissions (pool-limited, role-limited)
- **Removes dependency** on `pvesh` and `pveam` binaries

## What an attacker cannot do

Even if the service process is compromised:

- **Run arbitrary commands as root**: Sudoers only allows `nsenter --mount=/proc/1/ns/mnt --` with specific binaries (`pct exec/push/set`, `tee`, `mkdir`, `chown`). Substituting a different binary (e.g., `nsenter -- /bin/bash`) is rejected.
- **Escape other namespaces**: Only `--mount` is specified. PID, network, and user namespaces are unaffected.
- **Run nsenter without sudo**: `/proc/1/ns/mnt` is owned by root and inaccessible to the `appstore` user directly.
- **Modify the sudoers file**: `/etc/sudoers.d/` is protected by `ProtectSystem=strict` (read-only for the service).
- **Write to system binaries or config**: `/usr`, `/etc`, `/boot` are all read-only.
- **Access home directories**: `ProtectHome=yes` blocks `/home`, `/root`, `/run/user`.
- **Manage containers outside the pool**: API token permissions are scoped to the configured pool.

## GPU Passthrough

### Device passthrough

GPU-enabled apps declare profiles in their manifest (e.g. `nvidia-basic`, `dri-render`). At install time, the engine:

1. **Validates device nodes exist** on the host (e.g. `/dev/nvidia0`) before adding them to the job
2. If `gpu.required: false` and no GPU hardware is found, the install proceeds in CPU-only mode — no devices are passed through
3. If `gpu.required: true` and devices are missing, the install fails early with a clear error
4. Device passthrough is applied via `pct set` (see Shell operations above), not the Proxmox API

### NVIDIA library bind-mount

LXC containers share the host's kernel, so the host's NVIDIA kernel module is always in use. The userspace libraries (libcuda, libnvidia-ml, etc.) **must match the kernel module version exactly** — installing a different driver version inside the container will fail with version mismatch errors.

To solve this, the engine automatically bind-mounts the host's NVIDIA libraries into GPU containers:

1. Resolves the host library path — prefers the curated `/usr/lib/x86_64-linux-gnu/nvidia/current/` directory (Debian NVIDIA packages), falls back to auto-discovering libraries by glob
2. Adds a **read-only bind mount** via `pct set -mpN <host-path>,mp=/usr/lib/nvidia,ro=1`
3. After container start, creates `/etc/ld.so.conf.d/nvidia.conf` inside the container and runs `ldconfig` so applications can find the libraries

This approach:

| | Install driver in container | Bind-mount from host |
|---|---|---|
| Version match | Must manually match host kernel module | Always matches automatically |
| Disk usage | Full driver per container (~100 MB+) | Zero — shared from host |
| Host driver upgrade | Every container breaks until updated | All containers pick up new version on restart |
| Write access | Container can modify its own libs | Read-only — container cannot tamper with host libs |
| App author effort | Must handle driver installation | Transparent — engine handles it |

### What containers see

- `/dev/nvidia0`, `/dev/nvidiactl`, `/dev/nvidia-uvm` — device nodes (via `pct set -devN`)
- `/usr/lib/nvidia/` — read-only bind mount of host NVIDIA libraries
- `ldconfig` configured to search `/usr/lib/nvidia`
- Applications can use CUDA, NVML, and other NVIDIA APIs without any driver installation

### What containers cannot do

- **Modify host NVIDIA libraries**: The bind mount is read-only (`ro=1`)
- **Load a different driver version**: The kernel module is the host's; userspace must match
- **Access GPU devices not passed through**: Only explicitly configured devices are visible

## Provisioning SDK Permissions

Install scripts run inside the target LXC container via `pct exec`. The Python SDK enforces a permission model declared in each app's `app.yml` manifest — scripts cannot perform actions not listed in their `permissions:` block.

### Permission categories

| Category | Manifest key | What it controls |
|----------|-------------|------------------|
| System packages | `packages` | `pkg_install()` — only listed packages can be installed via apk/apt |
| Pip packages | `pip` | `pip_install()` — only listed pip packages can be installed |
| URLs | `urls` | `download()`, `add_apt_key()`, `add_apt_repo()` — only listed URLs (supports wildcards) |
| Installer scripts | `installer_scripts` | `run_installer_script()` — only listed script URLs can be downloaded and executed |
| APT repos | `apt_repos` | `add_apt_repository()` — only listed repository URLs can be added |
| Filesystem paths | `paths` | `create_dir()`, `create_venv()`, file operations — only under listed path prefixes |
| Commands | `commands` | `run_command()` — only listed binary names (supports wildcards) |
| Services | `services` | `enable_service()`, `create_service()`, `restart_service()` — only listed service names |
| Users | `users` | `create_user()` — only listed usernames |

### Implicitly allowed paths

The following paths are always permitted without manifest declaration:

| Path | Reason |
|------|--------|
| `/tmp` | Standard scratch space for downloads and temporary files |
| `/opt/venv` | Default Python virtual environment (see below) |

### Python virtual environments (PEP 668)

Modern Linux distributions (Alpine 3.22+, Debian 12+, Ubuntu 24.04+) mark the system Python as "externally managed" per PEP 668, refusing system-wide `pip install`. The SDK handles this transparently:

1. `pip_install()` auto-creates a venv at `/opt/venv` on first call
2. All pip packages are installed into the venv, never system-wide
3. The venv path is implicitly allowed (no manifest entry needed)
4. Apps can use `pip_install(venv="/opt/venv/myapp")` for explicit per-service isolation
5. `create_venv()` remains available for manual control (requires `paths` permission)

### Template files and configuration

App install scripts use template files stored in the `provision/` directory rather than embedding config content as inline strings. This approach:

- Keeps install scripts readable and auditable
- Separates configuration structure from provisioning logic
- Uses `render_template()` for template variable substitution (`$variable` syntax)
- Uses `deploy_provision_file()` to copy static files without modification
- Uses `provision_file()` to read template content for programmatic use

Template substitution supports `$variable` placeholders and conditional blocks (`{{#key}}...{{/key}}`). The engine pushes all provision files into the container at `/opt/appstore/provision/` before running the install script.

### Input type safety

The SDK provides typed input accessors that match the `type:` declared in `app.yml`:

| Manifest type | SDK method | Python type |
|--------------|-----------|-------------|
| `string` | `inputs.string(key, default)` | `str` |
| `number` | `inputs.integer(key, default)` | `int` |
| `boolean` | `inputs.boolean(key, default)` | `bool` |
| `secret` | `inputs.string(key, default)` | `str` |
| `select` | `inputs.string(key, default)` | `str` |

### Reconfigure endpoint

The `POST /api/installs/{id}/reconfigure` endpoint allows in-place changes to an installed app's settings without destroying the container. Only inputs marked `reconfigurable: true` in the manifest can be changed post-install. The endpoint:

1. Validates the request against the app's manifest (only reconfigurable inputs accepted)
2. Updates LXC resources (cores, memory) via the Proxmox API if changed
3. Runs the app's `configure()` Python method with updated inputs
4. Updates the install record in the database

Container-destructive changes (disk resize, bridge change, storage pool) require the full edit/rebuild flow.

### What a malicious install script cannot do

Even if an app's `install.py` is compromised:

- **Install unlisted packages**: `pkg_install("backdoor")` raises `PermissionDeniedError`
- **Download from arbitrary URLs**: `download("http://evil.com/...")` is rejected unless the URL matches a `urls` pattern
- **Write to arbitrary paths**: `create_dir("/etc/shadow")` fails — only declared path prefixes are allowed
- **Run arbitrary commands**: `run_command(["rm", "-rf", "/"])` fails — `rm` must be in `commands`
- **Create arbitrary users**: `create_user("root")` fails unless `root` is in `users`
- **Enable arbitrary services**: `enable_service("sshd")` fails unless `sshd` is in `services`
- **Escape the container**: Scripts run inside the LXC container via `pct exec`, not on the host

## Pool and Tag Boundaries

- The service only manages containers within its configured Proxmox pool
- All managed containers are tagged `appstore;managed`
- The service refuses to touch containers without this tag

## Authentication

### Session tokens

The web UI authenticates with a password configured during setup. On successful login, the server issues an HMAC-SHA256 signed session token stored as an `HttpOnly`, `SameSite=Lax` cookie. Tokens include an expiration timestamp and are verified on every authenticated request.

### Rate limiting

Login attempts are rate-limited to **5 attempts per minute per IP address**. After the limit is exceeded, further attempts are rejected with HTTP 429. The rate limiter uses the client's direct connection IP (`RemoteAddr`). The `X-Forwarded-For` header is **not trusted** — since the service typically runs with direct access (not behind a reverse proxy), trusting XFF would allow trivial rate-limit bypass by rotating header values.

### Terminal tokens

WebSocket terminal connections use ephemeral one-time tokens with a **30-second TTL**. The token is generated via an authenticated API call, then exchanged during the WebSocket handshake. Each token can only be used once and expires immediately after use or after 30 seconds.

## Developer Mode

Developer mode is gated behind two checks:

1. **Authentication** — all `/api/dev/*` endpoints require a valid session
2. **Feature flag** — the `developer.enabled` config flag must be `true`; the `withDevMode()` middleware rejects requests with HTTP 403 if developer mode is disabled

Developer mode allows creating, editing, and deploying custom apps but cannot modify the official catalog on disk — dev apps are stored separately in `/var/lib/pve-appstore/dev-apps/`.

## HTTP Security

### Request size limits

- **API requests** (POST/PUT/DELETE): 1 MB maximum body size
- **File uploads** (dev mode): 2 MB maximum body size

### CORS

The server validates `Origin` headers against allowed patterns:
- The configured host address and port
- `localhost` and `127.0.0.1` with any port (for local development)

Cross-origin requests from other origins are rejected.

### Path traversal protection

- File operations in developer mode validate paths with `filepath.Clean()` and reject paths containing `..`
- The filesystem browser restricts access to configured storage pool roots via prefix matching
- App provision files are confined to the app's `provision/` directory

### SQL injection

All database queries use parameterized queries with placeholder binding — no string interpolation of user input into SQL.

## Secrets

- API tokens and passwords are never logged
- Secrets are passed to containers via environment variables or temporary config files, never via shell command strings
- Configuration file (`/etc/pve-appstore/config.yml`) is owned `root:appstore` with mode `0640`
- TLS: Proxmox self-signed certs are accepted by default (`tls_skip_verify: true`); custom CA can be configured via `tls_ca_cert`
- Input values marked with `type: secret` in manifests are redacted from job logs when listed in `provisioning.redact_keys`
