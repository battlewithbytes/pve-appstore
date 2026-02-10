# PVE App Store

One-click LXC container apps for Proxmox VE. An "Unraid Apps"-style application store that provisions self-contained containers from a Git-based catalog with first-class GPU support.

## Features

- **One-click installs** — Browse, configure, and deploy apps from a web UI
- **Git-based catalog** — Apps defined as YAML manifests in a Git repository; stays up to date automatically
- **Sandboxed provisioning** — Python SDK with allowlist-enforced permissions; apps can only install packages, write files, and enable services they've declared
- **GPU passthrough** — Intel QSV and NVIDIA profiles for media transcoding and AI workloads
- **Proxmox-native** — Uses the Proxmox REST API for container lifecycle; runs as a systemd service
- **Unprivileged by default** — Containers run unprivileged with minimal sudo surface (pct exec/push only)

## Quick Start

**Requirements:** Proxmox VE 8.x+, single node.

```bash
# Download and run the interactive installer
curl -fsSL https://github.com/battlewithbytes/pve-appstore/releases/latest/download/install.sh | bash
```

The installer will:
1. Detect your Proxmox host and architecture
2. Download the latest release binary
3. Run the TUI setup wizard (Proxmox API token, catalog URL, network bridge, storage)
4. Create the systemd service and start the web UI

After setup, open the web UI to browse and install apps.

## Architecture

```
pve-appstore (Go binary)
  ├── Web UI (React SPA)          — browse, search, install apps
  ├── REST API                    — app catalog, install jobs, auth
  ├── Catalog module              — git clone/pull, manifest validation
  ├── Install engine              — job queue, CT provisioning pipeline
  ├── Proxmox API client          — container lifecycle, task polling
  └── Python SDK (embedded)       — sandboxed provisioning inside containers
```

The service runs as an unprivileged `appstore` user. Container operations use the Proxmox REST API with token-based auth. Provisioning scripts run inside containers via `pct exec`, with the embedded Python SDK enforcing permission boundaries.

## Development

```bash
# Install dependencies
make deps

# Run tests (Go + Python SDK)
make test
cd sdk/python && python3 -m pytest tests/

# Build binary
make build

# Run dev server with test catalog
make run-serve

# Build frontend
make frontend

# Cross-compile release binaries
make release
```

### Project Structure

```
cmd/pve-appstore/       CLI entry point (cobra)
internal/
  config/               config.yml parsing and validation
  catalog/              git catalog, manifest parsing, search
  server/               HTTP server, REST API, auth, SPA serving
  engine/               install/uninstall job pipeline
  proxmox/              Proxmox REST API client
  pct/                  pct exec/push wrappers
  installer/            TUI setup wizard
  version/              build version info
sdk/python/appstore/    Python provisioning SDK
web/frontend/           React + TypeScript SPA
deploy/                 install.sh one-liner
testdata/catalog/       sample app catalog for testing
```

## App Catalog

Apps live in a separate Git repository ([pve-appstore-catalog](https://github.com/battlewithbytes/pve-appstore-catalog)). Each app has a YAML manifest defining metadata, LXC defaults, user inputs, permissions, and a Python install script.

Use the web UI to browse and search the catalog — filter by name, category, or tags.

For details on writing apps, see the [App Development Tutorial](tutorial.md) or the [catalog README](https://github.com/battlewithbytes/pve-appstore-catalog).

## Security

- Apps declare permissions in their manifest; the SDK enforces them at runtime
- No arbitrary shell execution — all provisioning uses the Python SDK
- Containers are unprivileged by default with nesting enabled
- The service uses Proxmox API tokens (not root credentials)
- Pool and tag boundaries prevent touching unmanaged containers
- Secrets are never logged and are redacted in job output

See [SECURITY.md](SECURITY.md) for the full security model.

## License

Apache-2.0
