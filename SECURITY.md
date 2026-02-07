# Security Model

This document describes how PVE App Store runs on a Proxmox VE host with least-privilege design.

## Privilege Separation

The service runs as the unprivileged `appstore` Linux user under systemd. It never runs as root.

### Proxmox REST API (primary)

Container lifecycle operations (create, start, stop, shutdown, destroy, status) and cluster queries (next CTID, template listing) use the **Proxmox REST API** via an API token. This avoids privilege escalation entirely for these operations — the API token carries only the permissions granted to it.

### Shell operations (fallback)

Two operations have no REST API equivalent and require `sudo` via a strict sudoers allowlist:

- `pct exec` — run commands inside a container
- `pct push` — copy files from the host into a container

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
| `NoNewPrivileges=no` | Required to allow `sudo` for `pct exec`/`pct push` child processes |

All web server, API handler, catalog parsing, and general application code runs within this sandbox.

### Sudoers allowlist

Only two commands are permitted via `/etc/sudoers.d/pve-appstore`:

```
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct exec *
appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct push *
```

No other commands can be run as root by the `appstore` user.

## Mount Namespace Escape (nsenter)

### The problem

`ProtectSystem=strict` creates a read-only mount namespace for the service process. This namespace is **inherited by all child processes**, including those run via `sudo`. When `pct exec` runs as root, it still cannot write to paths like `/run/lock/lxc/` because the mount namespace makes them read-only.

### The solution

The two remaining privileged commands are wrapped with `nsenter --mount=/proc/1/ns/mnt --` which re-enters PID 1's (host init) mount namespace before executing the command:

```
sudo /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct exec 104 -- ...
```

This means:

- The **main service process stays fully sandboxed** — web server, API handlers, catalog parser, etc. all run under `ProtectSystem=strict`
- Only `pct exec` and `pct push` child processes escape to the host mount namespace
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

The API approach:
- **Reduces sudoers entries from 11 to 2**
- **Eliminates `sudo` privilege escalation** for most operations
- **Uses the API token** with scoped permissions (pool-limited, role-limited)
- **Removes dependency** on `pvesh` and `pveam` binaries

## What an attacker cannot do

Even if the service process is compromised:

- **Run arbitrary commands as root**: Sudoers only allows `nsenter --mount=/proc/1/ns/mnt -- /usr/sbin/pct exec *` and `pct push *`. Substituting a different binary (e.g., `nsenter -- /bin/bash`) is rejected.
- **Escape other namespaces**: Only `--mount` is specified. PID, network, and user namespaces are unaffected.
- **Run nsenter without sudo**: `/proc/1/ns/mnt` is owned by root and inaccessible to the `appstore` user directly.
- **Modify the sudoers file**: `/etc/sudoers.d/` is protected by `ProtectSystem=strict` (read-only for the service).
- **Write to system binaries or config**: `/usr`, `/etc`, `/boot` are all read-only.
- **Access home directories**: `ProtectHome=yes` blocks `/home`, `/root`, `/run/user`.
- **Manage containers outside the pool**: API token permissions are scoped to the configured pool.

## Pool and Tag Boundaries

- The service only manages containers within its configured Proxmox pool
- All managed containers are tagged `appstore;managed`
- The service refuses to touch containers without this tag

## Secrets

- API tokens and passwords are never logged
- Secrets are passed to containers via environment variables or temporary config files, never via shell command strings
- Configuration file (`/etc/pve-appstore/config.yml`) is owned `root:appstore` with mode `0640`
- TLS: Proxmox self-signed certs are accepted by default (`tls_skip_verify: true`); custom CA can be configured via `tls_ca_cert`
