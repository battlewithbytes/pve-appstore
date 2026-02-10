# PVE App Store

An app store for Proxmox VE — install self-hosted apps in one click.

PVE App Store lets you browse a catalog of self-hosted applications and deploy them as LXC containers on your Proxmox server, all from a web UI. Pick an app, tweak a few settings, and the store handles container creation, networking, and provisioning automatically.

![App Catalog](docs/screenshots/apps.png)

## Highlights

- **One-click install** from a web UI — no manual container setup or shell scripts
- **Growing catalog** of apps in a [separate Git repo](https://github.com/battlewithbytes/pve-appstore-catalog), from media servers to AI tools
- **Multi-app stacks** — export/import groups of apps as a single YAML file
- **GPU passthrough** — Intel QSV and NVIDIA profiles for transcoding and AI workloads
- **Config backup & restore** — save your installs and settings as portable YAML
- **Sandboxed provisioning** — apps run through a Python SDK with enforced permission boundaries

## Quick Start

**Prerequisites:** Proxmox VE 8.x on a single node.

```bash
curl -fsSL https://github.com/battlewithbytes/pve-appstore/releases/latest/download/install.sh | bash
```

The installer will:

1. Detect your Proxmox host and CPU architecture
2. Download the latest release binary
3. Walk you through a setup wizard (API token, catalog URL, storage, network bridge)
4. Create a systemd service and start the web UI

Open **http://your-proxmox-ip:8088** to browse and install apps.

## Available Apps

| App | Category | GPU |
|-----|----------|-----|
| Crawl4AI | AI | — |
| Gluetun VPN Client | Networking | — |
| Home Assistant | Automation | — |
| Jellyfin | Media | Intel / NVIDIA |
| Nginx | Web | — |
| Ollama | AI | Intel / NVIDIA |
| Plex Media Server | Media | Intel / NVIDIA |

See the [catalog repo](https://github.com/battlewithbytes/pve-appstore-catalog) for the full list and details.

## Screenshots

### Installed Apps

Live status, resource bars, network links, and uptime — all refreshed from the Proxmox API.

![Installed Apps](docs/screenshots/installed.png)

### Install Detail

Per-container metrics (CPU, memory, disk, network), mount points, service URLs, and container config at a glance.

![Install Detail](docs/screenshots/details.png)

### Web Terminal

Drop into any container shell directly from the browser.

![Web Terminal](docs/screenshots/shell.png)

### Multi-App Stacks

Bundle multiple apps into a single container with a step-by-step wizard.

![Create Stack — Apps](docs/screenshots/createstack.png)
![Create Stack — Resources](docs/screenshots/stack.png)

### Config Export & Restore

Back up all installs and stacks as portable YAML, then restore on another node.

![Configuration](docs/screenshots/config.png)

## How It Works

The catalog is a Git repository of app manifests (YAML + Python install scripts). PVE App Store clones it locally and serves the catalog through a web UI. When you install an app, a job engine creates an LXC container via the Proxmox REST API, pushes the install script inside, and runs it through a sandboxed Python SDK that enforces declared permissions.

## Writing Your Own App

App manifests are YAML files paired with a Python install script. The [App Development Tutorial](tutorial.md) walks through building one from scratch, and the [catalog repo](https://github.com/battlewithbytes/pve-appstore-catalog) has a quickstart guide.

## Security

- Runs as an unprivileged `appstore` user under systemd with `ProtectSystem=strict`
- Uses Proxmox API tokens — never root credentials — scoped to a single pool
- Provisioning SDK enforces per-app permission allowlists; no arbitrary shell execution

See [SECURITY.md](SECURITY.md) for the full security model.

## Development

```bash
make deps          # install Go + JS dependencies
make build         # compile binary with version info
make test          # Go tests + Python SDK tests
make frontend      # build React SPA
make run-serve     # dev server with test catalog
make release       # cross-compile linux/amd64 + arm64
```

### Project Structure

```
cmd/pve-appstore/        CLI entry point (cobra)
internal/
  config/                config.yml parsing and validation
  catalog/               git catalog, manifest parsing, search
  server/                HTTP server, REST API, auth, SPA serving
  engine/                install/uninstall job pipeline
  proxmox/               Proxmox REST API client
  pct/                   pct exec/push wrappers
  installer/             TUI setup wizard
sdk/python/appstore/     Python provisioning SDK
web/frontend/            React + TypeScript SPA
deploy/                  install.sh one-liner
testdata/catalog/        sample app catalog for testing
```

## License

Apache-2.0
