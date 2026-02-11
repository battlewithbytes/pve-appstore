# Resilio Sync

Fast peer-to-peer file synchronization using BitTorrent technology, running in a Proxmox LXC container.

## Features

- **Peer-to-peer sync** — files transfer directly between devices, no cloud required
- **Selective sync** — choose which folders and files to sync per device
- **Encrypted folders** — share read-only encrypted copies with untrusted peers
- **Web UI** — manage folders, peers, and settings from any browser
- **No file size limits** — sync files of any size

## Requirements

- 2 cores, 1 GB RAM, 8 GB disk (minimum)
- Debian 12 LXC container

## Configuration

| Input | Default | Reconfigurable | Description |
|-------|---------|---------------|-------------|
| Bind Address | 0.0.0.0 | Yes | Web UI listen address |
| Web UI Port | 8888 | Yes | Web interface port |
| Listening Port | 55555 | Yes | P2P sync port |

## Post-Install

1. Open the Web UI at the URL shown in outputs
2. Create a username and password on first access
3. Add folders to sync and generate share links/keys
