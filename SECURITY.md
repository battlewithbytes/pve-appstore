# Security Model

This document describes how PVE App Store runs on a Proxmox VE host with least-privilege design.

## Privilege Separation

The service runs as the unprivileged `appstore` Linux user under systemd. It never runs as root.

### Proxmox REST API (primary)

Container lifecycle operations (create, start, stop, shutdown, destroy, status) and cluster queries (next CTID, template listing) use the **Proxmox REST API** via an API token. This avoids privilege escalation entirely for these operations — the API token carries only the permissions granted to it.

### Helper Daemon (privileged operations)

Eight operations have no REST API equivalent (or are restricted to `root@pam` in the API) and require root access. These are handled by a dedicated **helper daemon** (`pve-appstore-helper`) that runs as root and exposes a validated HTTP REST API over a Unix domain socket at `/run/pve-appstore/helper.sock`.

The main service connects to the helper socket; the helper performs the privileged operation. All requests are structured JSON — no shell argument injection is possible.

| Operation | Helper Endpoint | Why root is needed |
|-----------|----------------|-------------------|
| `pct exec` | `POST /v1/pct/exec` | No API endpoint exists for LXC command execution |
| `pct push` | `POST /v1/pct/push` | No API endpoint exists for LXC file push |
| `pct set` (devices, bind mounts) | `POST /v1/pct/set` | API restricts `dev*` and bind mounts to `root@pam` |
| LXC config append | `POST /v1/conf/append` | `/etc/pve/lxc/` is a FUSE-mounted cluster filesystem requiring root |
| mkdir | `POST /v1/fs/mkdir` | Storage pool directories are owned by root |
| chown | `POST /v1/fs/chown` | Ownership change for unprivileged container UID mapping |
| rm | `POST /v1/fs/rm` | Clean up bind mount directories during uninstall |
| Self-update | `POST /v1/update` | Binary replacement and service restart |

The helper daemon validates every request at the privilege boundary before executing it.

### Helper Daemon Security

The helper daemon implements defense-in-depth at the privilege boundary:

**Socket access control:**
- Socket file: `/run/pve-appstore/helper.sock` owned `root:appstore` with mode `0660`
- Socket directory: `/run/pve-appstore/` owned `root:appstore` with mode `0750`
- SO_PEERCRED verification: every request's peer UID is verified against the `appstore` user

**CTID ownership verification:**
- Every CTID-accepting endpoint checks the install database (read-only connection) to verify the CTID belongs to a managed container
- The helper's DB connection is read-only — a compromised main service cannot insert fake records

**pct set option allowlist:**
- Only `-dev[0-9]{1,2}` (device passthrough) and `-mp[0-9]{1,2}` (bind mounts) are accepted
- All other options (`-rootfs`, `-ostype`, `-net`, `-memory`, etc.) are rejected
- Device values are validated against the same allowed device patterns used by the engine
- Bind mount host paths are validated against storage roots with symlink resolution

**Path validation (filesystem operations):**
- All paths are cleaned (`filepath.Clean`), resolved (`filepath.EvalSymlinks` on parent), and checked against storage root prefixes
- Deny-list blocks operations on `/etc`, `/proc`, `/sys`, `/dev`, `/root`, `/boot`, `/usr`, `/bin`, `/sbin`, `/lib`
- Symlinks at leaf paths are resolved and re-validated to prevent symlink attacks
- Push source files must be under `/var/lib/pve-appstore/tmp/` and must be regular files

**LXC config value validation:**
- Config keys are allowlisted: `lxc.cgroup2.devices.allow`, `lxc.mount.entry`, `lxc.mount.auto`, `lxc.environment`, `lxc.cgroup2.cpuset.cpus`
- Explicitly rejected: `lxc.apparmor.profile`, `lxc.seccomp.profile`, `lxc.cap.drop`, `lxc.cap.keep`, `lxc.rootfs`, `lxc.idmap`, `lxc.init.cmd`
- Values are validated: `cgroup2.devices.allow` must specify device type + major:minor (no `a` for allow-all), `mount.auto` must be from a safe list
- Config file path is constructed server-side from the CTID (not passed by the client)

**Chown UID/GID allowlist:**
- Only `100000:100000` (standard unprivileged container root mapping) is permitted

**Concurrency controls:**
- Per-CTID mutex for config-modifying operations
- Max 5 concurrent terminal sessions
- Max 20 concurrent exec operations
- 1 MB request body size limit on all endpoints

**Audit logging:**
- Every request logged as structured JSON to `/var/log/pve-appstore/helper-audit.log`
- Log is owned `root:root 0640` — the `appstore` user cannot modify or delete it
- Entries include: timestamp, peer UID/PID, endpoint, CTID, result, duration

**systemd hardening on the helper:**

| Directive | Effect |
|-----------|--------|
| `RestrictAddressFamilies=AF_UNIX` | Helper **cannot make network connections** — TCP/UDP sockets are blocked |
| `IPAddressDeny=any` | Additional network restriction |
| `NoNewPrivileges=yes` | No further privilege escalation |
| `ProtectHome=yes` | No access to home directories |
| `PrivateTmp=yes` | Isolated /tmp |
| `ProtectKernelModules=yes` | Cannot load kernel modules |
| `ProtectKernelTunables=yes` | Cannot modify kernel parameters |
| `SystemCallFilter=@system-service @mount @privileged` | Restricted syscall set |
| `SystemCallArchitectures=native` | Only native architecture syscalls |

### Unprivileged service process

The `pve-appstore.service` unit applies systemd sandboxing:

| Directive | Effect |
|-----------|--------|
| `User=appstore` | Process runs as unprivileged user |
| `ProtectSystem=strict` | `/usr`, `/boot`, `/efi`, `/etc` are read-only |
| `ProtectHome=yes` | `/home`, `/root`, `/run/user` are inaccessible |
| `PrivateTmp=yes` | Service gets an isolated `/tmp` |
| `ReadWritePaths=` | Only `/var/lib/pve-appstore` and `/var/log/pve-appstore` are writable |
| `NoNewPrivileges=yes` | No privilege escalation possible — sudo is not used |
| `Requires=pve-appstore-helper.service` | Helper daemon must be running |

All web server, API handler, catalog parsing, and general application code runs within this sandbox.

### Sudoers (legacy fallback)

A sudoers file is installed as a legacy fallback for environments where the helper daemon is not deployed. When the helper daemon socket is detected at startup, all privileged operations are routed through the helper and sudo is never invoked.

The sudoers fallback will be removed in a future release once the helper daemon has been validated across all deployment environments.

## Self-Update

### CLI update (`pve-appstore self-update`)

The CLI command runs as root directly. It:
1. Checks the GitHub Releases API for the latest version
2. Downloads the binary for the current architecture
3. Removes the running binary (Linux keeps the inode alive for the running process)
4. Moves the new binary into place at `/opt/pve-appstore/pve-appstore`
5. Restarts the systemd service

### Web update (`POST /api/system/update`)

The web endpoint runs as the `appstore` user. It:
1. Downloads the new binary to `/var/lib/pve-appstore/pve-appstore.new`
2. Delegates to the helper daemon's `POST /v1/update` endpoint
3. The helper validates the binary path (hardcoded, not from request), verifies it's a regular file
4. Runs the update script detached via `setsid` so it survives the service restart
5. The script replaces the binary and restarts both the helper and main service

### Update check endpoint

`GET /api/system/update-check` queries the GitHub Releases API (public, no auth needed) and compares the response with the running version using semver parsing. Results are cached in-memory for 1 hour to avoid rate limiting. The endpoint requires authentication (via `withAuth` middleware).

## API vs Shell attack surface

| Operation | Method | Privilege |
|-----------|--------|-----------|
| Container create/start/stop/destroy | REST API via token | 0 sudoers entries |
| CTID allocation | REST API `GET /cluster/nextid` | 0 sudoers entries |
| Template listing | REST API `GET /nodes/{node}/storage/{storage}/content` | 0 sudoers entries |
| GPU detection | REST API `GET /nodes/{node}/hardware/pci` | 0 sudoers entries |
| Container exec | Helper `POST /v1/pct/exec` | Validated JSON, CTID verified |
| File push | Helper `POST /v1/pct/push` | Source path validated, CTID verified |
| Device passthrough | Helper `POST /v1/pct/set` | Option allowlist, device path allowlist |
| Host directory creation | Helper `POST /v1/fs/mkdir` | Path validation with symlink resolution |
| Bind mount ownership | Helper `POST /v1/fs/chown` | UID/GID allowlist, path validation |
| Extra LXC config | Helper `POST /v1/conf/append` | Key+value validation, path server-constructed |
| Self-update | Helper `POST /v1/update` | Binary path hardcoded, regular file check |

## What an attacker cannot do

Even if the service process is compromised:

- **Send malformed arguments to privileged commands**: All inputs are parsed from structured JSON, not shell-interpolated argv
- **Target containers outside the managed set**: The helper verifies every CTID against the install database
- **Use dangerous pct set options**: Only `-dev[N]` and `-mp[N]` are permitted — no `-rootfs`, `-ostype`, etc.
- **Inject arbitrary LXC config lines**: Config keys and values are validated against strict allowlists
- **Traverse paths via symlinks**: All paths are resolved and validated against storage roots and a deny-list
- **Make the helper daemon talk to the network**: `RestrictAddressFamilies=AF_UNIX` prevents TCP/UDP socket creation
- **Access the helper socket**: It's owned `root:appstore 0660`, and the helper verifies peer UID via SO_PEERCRED
- **Modify the audit log**: Owned `root:root 0640`, inaccessible to the `appstore` user
- **Write to system binaries or config**: `/usr`, `/etc`, `/boot` are all read-only via `ProtectSystem=strict`
- **Access home directories**: `ProtectHome=yes` blocks `/home`, `/root`, `/run/user`
- **Manage containers outside the pool**: API token permissions are scoped to the configured pool
- **Escalate privileges**: `NoNewPrivileges=yes` on both the main service and the helper

## GPU Passthrough

### GPU detection

GPU hardware is detected by combining two sources:

1. **Device node enumeration**: `/dev/dri/renderD*` (DRI render nodes) and `/dev/nvidia[0-9]*` (NVIDIA device nodes) are discovered via filesystem glob
2. **PCI device identification**: The Proxmox REST API (`GET /nodes/{node}/hardware/pci`) provides vendor names, device names, PCI class codes, and IOMMU group info for accurate GPU identification

The sysfs symlink at `/sys/class/drm/{node}/device` resolves each DRI render node to its PCI address (e.g. `0000:61:00.0`), which is then matched against the API response for rich device info. NVIDIA device nodes are matched via PCI addresses from `/sys/bus/pci/drivers/nvidia/`.

The web server (running as `appstore` user) uses the Proxmox REST API via the existing API token. The TUI installer (running as root) falls back to `pvesh` for the same data. If the API is unavailable, device nodes still appear with generic names derived from sysfs vendor IDs.

The API token requires `Sys.Audit` permission at the root path (`/`) for the PCI hardware endpoint. This is granted via the `PVEAuditor` role, which is read-only and cannot modify any system state.

### Driver status detection

The GPU API endpoint (`GET /api/system/gpus`) includes a `driver_status` object that reports the state of GPU kernel drivers and userspace libraries on the host:

- **NVIDIA**: Checks `/sys/module/nvidia` for kernel module presence, reads `/sys/module/nvidia/version` for the driver version, and probes for userspace libraries in well-known host paths
- **Intel**: Checks `/sys/module/i915` or `/sys/module/xe` (newer Intel GPUs)
- **AMD**: Checks `/sys/module/amdgpu`

The install form and settings page display driver warnings when hardware is detected but drivers are missing, helping users resolve issues before installing GPU-enabled apps.

### Device passthrough

The install form auto-detects GPUs on the system and presents each as a selectable checkbox. The user selects which GPUs to pass through, and the engine maps each GPU type to the correct device nodes:

- **Intel/AMD**: The render node (e.g. `/dev/dri/renderD128`) with GID 44 and mode `0666`
- **NVIDIA**: The device node (e.g. `/dev/nvidia0`) plus shared control nodes (`/dev/nvidiactl`, `/dev/nvidia-uvm`)

At install time, the engine:

1. **Validates device nodes exist** on the host (e.g. `/dev/nvidia0`) before adding them to the job
2. If `gpu.required: false` and no GPU hardware is found, the install proceeds in CPU-only mode — no devices are passed through
3. If `gpu.required: true` and devices are missing, the install fails early with a clear error
4. Device passthrough is applied via the helper daemon's `POST /v1/pct/set` endpoint with strict device path validation

### NVIDIA library bind-mount

LXC containers share the host's kernel, so the host's NVIDIA kernel module is always in use. The userspace libraries (libcuda, libnvidia-ml, etc.) **must match the kernel module version exactly** — installing a different driver version inside the container will fail with version mismatch errors.

To solve this, the engine automatically bind-mounts the host's NVIDIA libraries into GPU containers:

1. Resolves the host library path — prefers the curated `/usr/lib/x86_64-linux-gnu/nvidia/current/` directory (Debian NVIDIA packages), falls back to auto-discovering libraries by glob
2. Adds a **read-only bind mount** via the helper daemon's `POST /v1/pct/set` with `-mpN`
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
- **Helper daemon requests**: 1 MB maximum body size

### CORS

The server validates `Origin` headers against allowed patterns:
- The configured host address and port
- `localhost` and `127.0.0.1` with any port (for local development)

Cross-origin requests from other origins are rejected.

### Path traversal protection

- File operations in developer mode validate paths with `filepath.Clean()` and reject paths containing `..`
- The filesystem browser restricts access to configured storage pool roots via prefix matching
- The helper daemon validates all paths with `EvalSymlinks` on parent directories and deny-list checks
- App provision files are confined to the app's `provision/` directory

### SQL injection

All database queries use parameterized queries with placeholder binding — no string interpolation of user input into SQL.

## Secrets

- API tokens and passwords are never logged
- Secrets are passed to containers via environment variables or temporary config files, never via shell command strings
- Configuration file (`/etc/pve-appstore/config.yml`) is owned `root:appstore` with mode `0640`
- TLS: Proxmox self-signed certs are accepted by default (`tls_skip_verify: true`); custom CA can be configured via `tls_ca_cert`
- Input values marked with `type: secret` in manifests are redacted from job logs when listed in `provisioning.redact_keys`
