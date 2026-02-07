# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PVE App Store: an "Unraid Apps"-style application store for Proxmox VE. It provisions self-contained LXC containers from a Git-based catalog with first-class GPU support (Intel/AMD iGPU + NVIDIA).

**Status:** Pre-implementation. The repository currently contains only the PRD (`appstore-prd.md`). All code must be built from scratch following that spec.

**Target platform:** Proxmox VE 8.x+, single-node deployment (v0).

## Architecture (from PRD)

The system is a **node-local systemd service** (`pve-appstore.service`) running as an unprivileged `appstore` Linux user. It has these core modules:

1. **Config module** - parses `/etc/pve-appstore/config.yml`, validates, optional hot-reload
2. **Catalog module** - git-fetches app catalog, validates manifests against schema, builds search index
3. **Proxmox API client** - token-based auth, discovery (pools/storage/bridges), CT lifecycle (create/config/start/stop)
4. **Host ops module** - safe `pct` wrappers with strict argv validation; all privileged ops via sudo allowlist
5. **Job engine** - SQLite-backed state machine (16 states from `queued` to `completed`/`failed`), timeouts, retries, log streaming
6. **GPU manager** - device discovery (`/dev/dri/*`, `/dev/nvidia*`), profile-based attachment, cgroup v2 device rules
7. **Web API** - REST endpoints + optional WebSocket/SSE for log streaming, session-cookie auth (v0)
8. **Web UI** - static SPA served by the controller

### Filesystem Layout (host)

- `/opt/pve-appstore/` - application binaries/assets
- `/etc/pve-appstore/config.yml` - configuration (root:appstore 0640)
- `/var/lib/pve-appstore/` - state: `jobs.db` (SQLite), cached catalog, temp files
- `/var/log/pve-appstore/` - structured JSON logs with rotation

### Data Model (SQLite)

Tables: `apps`, `installs`, `jobs`, `job_logs`, `gpu_attachments`. See PRD section 9.2 for schemas.

### App Catalog Structure

```
catalog/apps/<app-id>/
  app.yml              # manifest (metadata, placement defaults, inputs, provisioning, GPU, outputs)
  icon.png             # optional
  README.md            # optional
  provision/
    install.sh         # required
    upgrade.sh         # optional
    uninstall.sh       # optional
    healthcheck.sh     # optional
  templates/*.tmpl     # optional
```

## Security Model (Critical)

These constraints must be enforced in all code:

- **Pool boundary:** only manage CTs in the configured Proxmox pool
- **Tag boundary:** all managed CTs tagged `appstore:managed=1`; refuse to touch untagged CTs
- **No arbitrary commands:** no `bash -c`, no shell strings; provisioning only via catalog scripts
- **Privilege separation:** service runs unprivileged; `pct` operations via sudo allowlist with explicit binary paths
- **Secrets:** never logged, redacted in job outputs, passed via env vars or temp config files (not shell strings)
- **Proxmox API:** use API tokens (not tickets), correct HTTP verbs per endpoint (PUT vs POST matters; 501 often means wrong method)

## GPU Profiles

- **`dri-render`** (Intel/AMD): bind-mount `/dev/dri`, allow cgroup char major 226
- **`nvidia-basic`**: bind-mount `/dev/nvidia0`, `/dev/nvidiactl`, `/dev/nvidia-uvm`, `/dev/nvidia-uvm-tools`; detect majors dynamically; validate with `nvidia-smi`

## Implementation Milestones (v0)

0. Repo scaffolding, config, systemd template, installer TUI
1. Catalog fetch, manifest validation, app browsing UI
2. Install engine: CT provisioning via `pct exec`, logs, outputs
3. Security hardening: pool/tag enforcement, sudo allowlist, secret redaction
4. GPU v0: discovery, `dri-render` profile, manifest gating, UI
5. Obsidian LiveSync as reference catalog app
