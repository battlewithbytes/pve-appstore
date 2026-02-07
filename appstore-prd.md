# PVE App Store — PRD (Native Service + LXC “App Store” with GPU Support)

**Document name:** `prd.md`  
**Status:** Draft (implementation-ready)  
**Target platform:** Proxmox VE 8.x+ (single “management node” deployment for v0)  
**Last updated:** 2026-02-07

---

## 0. Executive summary

Build an “Unraid Apps”-style App Store for Proxmox VE that:
- runs **natively as a systemd service on one Proxmox node** (no Docker, no SSH-to-host required for v0),
- provisions **self-contained LXC containers** from a Git-based catalog (manifests + scripts),
- renders a **dynamic install UI** from app manifests (inputs, defaults, validation),
- runs an **install job engine** with live logs, retry/rollback controls,
- treats **GPUs as a first-class resource** (Intel/AMD iGPU via `/dev/dri`, NVIDIA via `/dev/nvidia*`, profiles + validation),
- enforces strict safety boundaries via **Proxmox Pool + tags**, least-privilege API tokens, and a local allowlist for privileged operations.

v0 scope is intentionally **single-node for “exec/provisioning”** to avoid SSH/agents: the service installs apps only onto the node it runs on. Cluster-wide provisioning is addressed in roadmap via optional template-based installs or an opt-in worker model.

---

## 1. Goals and non-goals

### 1.1 Goals
1. **Unraid-like UX on Proxmox**
   - Browse app catalog, search/filter, view details, click install, fill form, watch logs, open result URL.
2. **Native + secure**
   - Install controller as a node-local systemd service via a one-liner installer that asks guided questions (TUI).
3. **Manifest-driven apps**
   - Apps are defined via a structured manifest (`app.yml`) + provision scripts/assets.
4. **Self-contained LXC apps**
   - Each app installs into a single LXC by default (Obsidian LiveSync example: CouchDB + optional proxy all in one CT).
5. **GPU support is first-class**
   - App manifests can declare GPU capability requirements; installer/UI supports selecting and attaching GPUs.
6. **Operational safety**
   - Hard boundaries: only manage containers in a selected **Pool** and tagged as appstore-managed.
   - Auditable jobs and logs; secrets redacted; deterministic installs.

### 1.2 Non-goals (v0)
- Modifying/patching Proxmox’s built-in web UI.
- Arbitrary command execution requested by the user.
- Managing containers outside the configured Pool / tag boundary.
- Cluster-wide provisioning that requires remote `pct exec` on other nodes (no SSH/agent in v0).
- Multi-container “stacks” spanning multiple LXCs (future).

---

## 2. Personas and primary use cases

### 2.1 Personas
- **Homelab Admin:** wants “install Plex/Immich/Ollama/etc” without hand-writing LXC configs.
- **Maker/Dev:** wants repeatable installs, versioned manifests, and GPU enablement for AI workloads.
- **Catalog Maintainer:** writes/maintains app definitions; wants CI validation and reproducible provisioning.

### 2.2 Primary use cases
1. Install an app into a new LXC with a few inputs.
2. Install an app that requires GPU (e.g., Ollama/Whisper/Jellyfin) and attach the correct GPU device(s).
3. View install logs and outputs (URLs, credentials).
4. Uninstall an app (stop + destroy container and associated resources).
5. Upgrade/re-provision an app (rerun provision script with compatibility rules).

---

## 3. Product shape

### 3.1 Components
**A) Node-local Controller Service (v0)**
- Runs on one Proxmox node.
- Provides:
  - Web UI (static frontend)
  - REST API (backend)
  - Job engine + log streamer
  - Catalog fetch + validation

**B) App Catalog (Git repo)**
- Contains app directories with:
  - `app.yml` (manifest)
  - `icon.png` (optional)
  - `README.md` (optional)
  - `provision/` scripts
  - `templates/` or assets

**C) Proxmox integration**
- Uses Proxmox API for discovery + container CRUD where possible (token auth format documented). citeturn0search2turn0search10
- Uses local host tooling for operations not well supported via API (notably provisioning inside CT via `pct exec`).

---

## 4. Deployment model (native service)

### 4.1 Installation entrypoint
User runs one of:
- **One-liner:** `curl -fsSL <installer-url> | bash`
- Optional: `wget -qO- <installer-url> | bash`

Installer requirements:
- Works on Proxmox host (Debian-based).
- Provides a TUI if possible; falls back to simple prompts.
- Creates a dedicated Linux user (`appstore`) and systemd unit (`pve-appstore.service`).

### 4.2 Filesystem layout (host)
- `/opt/pve-appstore/` — application binaries/assets
- `/etc/pve-appstore/config.yml` — configuration (owned root:appstore; 0640)
- `/var/lib/pve-appstore/` — state: `jobs.db`, cached catalog, temp files
- `/var/log/pve-appstore/` — structured logs (rotate)

### 4.3 Runtime privileges
- Service runs as Linux user: `appstore`
- Privileged operations happen via **sudo allowlist** (no shell):
  - `pct` operations required for provisioning and finalization
  - minimal `/usr/bin/git` (optional; can be replaced with pure libgit)
- Any call to sudo is mediated by code-side argument validation.

---

## 5. Installer (guided TUI) requirements

### 5.1 TUI dependencies
Prefer:
- `gum` (best UX), else `dialog`, else `fzf`, else plain stdin prompts.
Installer can attempt to install `gum` automatically (optional toggle).

### 5.2 Questions to ask (v0)
**(A) Safety boundary**
1. **Proxmox Pool selection (required)**
   - List pools via API and allow select or “create new pool (default: appstore)”.
   - Hard rule: only manage LXCs in this pool.

**(B) Container placement defaults**
2. **Default Storage** for container rootfs (required)
   - List storages that support `rootdir`.
3. **Default Network Bridge** (required)
   - List `vmbr*` interfaces on the local node.

**(C) Default limits**
4. **Default resource caps**
   - cores, memory (MB), disk (GB)
   - These are defaults and optional upper bounds.

**(D) Security posture**
5. **Unprivileged containers only? (recommended yes)**
6. **Allowed LXC features** (checkbox)
   - `nesting` default ON
   - `keyctl`, `fuse` default OFF

**(E) Service exposure**
7. **UI bind address** (default `0.0.0.0`)
8. **UI port** (default `8088`)
9. **UI authentication**
   - none / password login (recommended) / OIDC (future)

**(F) Proxmox auth setup**
10. **Create dedicated Proxmox user + API token automatically? (recommended yes)**
    - If yes: generate token and store in config.
    - If no: prompt user for token id + secret.

Proxmox token usage must follow documented header format. citeturn0search2turn0search6turn0search10

**(G) Catalog**
11. Catalog repo URL (default “official”)
12. Branch (default `main`)
13. Auto-refresh schedule (daily/weekly/manual)

**(H) GPU awareness**
14. Detect GPUs on host and ask:
   - “Enable GPU support features in App Store?” (default yes)
   - “Default GPU access policy: none / allow (opt-in per-app) / allowlist devices”
   - If allowlist: choose GPU devices to allow attaching

### 5.3 Installer actions
- Create Linux user: `appstore`
- Write `/etc/pve-appstore/config.yml`
- Install sudoers file `/etc/sudoers.d/pve-appstore` (validate with `visudo -cf`)
- Install systemd unit file `/etc/systemd/system/pve-appstore.service`
- Start service and print URL + health endpoint

---

## 6. Security model (must be “boring and correct”)

### 6.1 Hard safety boundaries
- **Pool boundary:** service only manages CTs inside selected pool.
- **Tag boundary:** service marks all created CTs with tag `appstore:managed=1` and refuses to touch CTs without it.
- **Node boundary (v0):** service only provisions/executes jobs on its local node.

### 6.2 Proxmox permissions (least privilege)
- Create a dedicated Proxmox user (e.g., `appstore@pve`).
- Create a custom role with only required privileges for:
  - reading cluster/node resources
  - creating/configuring LXC containers in the chosen pool
  - starting/stopping those containers
- Apply role permissions only on:
  - the pool path (preferred)
  - and minimal `/nodes/<localnode>` read permissions

Implementation note: Proxmox API has strict HTTP methods per endpoint; use API Viewer and correct verbs (e.g., PUT vs POST where required). citeturn0search15

### 6.3 Local host privilege separation
- The service process runs unprivileged (`appstore` user).
- A privileged surface is exposed only via sudo allowlist with:
  - explicit binary paths
  - no `NOPASSWD: ALL`
  - no shells

**Allowed commands (conceptual list; finalize in implementation):**
- `/usr/sbin/pct create ...`
- `/usr/sbin/pct start|stop|shutdown|destroy <ctid>`
- `/usr/sbin/pct exec <ctid> -- <argv...>` (with strict argv policy)
- `/usr/sbin/pct push <ctid> <src> <dst> --perms ...` (optional)
- `/usr/sbin/pct status <ctid>`
- `/usr/sbin/pct config <ctid>` (read)
- `/usr/bin/pvesh ...` (read-only) or direct API calls (preferred)

### 6.4 No arbitrary commands
- UI never accepts “custom commands”.
- Provisioning is limited to:
  - scripts shipped in the catalog
  - pinned by hash
  - executed with structured inputs (env/config file)
- Code must reject any attempt to execute `bash -c ...` or other shell strings.

### 6.5 Secrets handling
- Secrets are accepted via UI as `secret` inputs.
- Secrets are:
  - stored encrypted-at-rest in the job record (optional for v0; recommended)
  - never logged
  - redacted in job output views
- For provisioning, secrets are passed via:
  - env vars (only to the exec call, not stored in logs)
  - or an injected config file with tight permissions, then deleted.

### 6.6 Auditability
- Every job records:
  - user action
  - manifest version + content hash
  - CTID, pool, node
  - GPU devices attached
  - full state transitions
- Logs are structured JSON with correlation IDs.

---

## 7. Functional requirements

### 7.1 Catalog & manifests
- Support catalog as a Git repo.
- Controller pulls and caches it locally.
- Validate each app manifest against schema.
- Render UI forms from `inputs`.
- Allow versioning and compatibility constraints.

### 7.2 App lifecycle
- Install → Create CT → Start → Provision → Verify → Outputs
- Uninstall → Stop → Destroy CT → Cleanup
- Reconfigure → Edit inputs → Apply (limited) → Restart (optional)
- Upgrade → Pull new catalog → compare manifest versions → apply upgrade plan

### 7.3 Job system
- Jobs persisted in SQLite (v0).
- State machine with retries and timeouts.
- Concurrency controls:
  - one active job per CTID
  - optional one active install at a time (configurable)

### 7.4 GPU as first-class citizen
- Catalog can declare:
  - “GPU optional” (can run without)
  - “GPU required” (block install unless GPU available)
  - Supported GPU types (intel/amd/nvidia)
  - Required runtime libraries (CUDA, VAAPI, ROCm)
- UI supports:
  - “No GPU”
  - “Auto-select best GPU”
  - “Pick device(s)” (advanced)
- Installer supports selecting a default GPU policy.

---

## 8. Non-functional requirements

- **Reliability:** Job engine is restart-safe (resume based on persisted state).
- **Performance:** Catalog refresh < 5s typical; installs stream logs in real time.
- **Compatibility:** Proxmox VE 8.x+; handle cgroupv2 device controls.
- **Security:** minimal privileges; no remote shell; strict allowlists.
- **Usability:** install is guided; UI is “Unraid-like” with sane defaults.

---

## 9. Detailed system design

## 9.1 Controller internal modules (suggested)
1. **Config module**
   - parse config.yml, validate, hot-reload optional
2. **Catalog module**
   - git pull / fetch
   - validate schema
   - build search index (tags/categories)
3. **Proxmox API client**
   - token auth
   - discovery endpoints
   - CT lifecycle endpoints
4. **Host ops module**
   - safe wrappers for `pct` and local file edits
   - full argv validation
5. **Job engine**
   - state machine
   - timeouts, retries
   - log streaming
6. **GPU manager**
   - detect devices
   - attach/detach profiles
   - validate container access
7. **Web API**
   - endpoints for UI
   - auth middleware
8. **Web UI**
   - SPA + server-served static assets

---

## 9.2 Data model (SQLite v0)

### Tables
**apps**
- `id`, `name`, `version`, `categories`, `tags`, `manifest_hash`, `path`, `icon_path`

**installs**
- `install_id`, `app_id`, `ctid`, `node`, `pool`, `status`, `created_at`, `updated_at`

**jobs**
- `job_id`, `type` (install/uninstall/upgrade), `state`, `ctid`, `app_id`, `manifest_hash`, `inputs_json`, `secrets_ref`, `created_at`, `updated_at`, `error`

**job_logs**
- `job_id`, `ts`, `level`, `message`, `json_blob`

**gpu_attachments**
- `ctid`, `profile`, `devices_json`, `created_at`

---

## 9.3 API surface (controller backend)

### Auth
- v0: optional password login; session cookie
- v1+: OIDC optional

### Endpoints (representative)
- `GET /api/health`
- `GET /api/apps` (search/filter)
- `GET /api/apps/{id}`
- `POST /api/install` (app_id + inputs + placement overrides + gpu selection)
- `GET /api/jobs/{job_id}`
- `GET /api/jobs/{job_id}/logs` (poll)
- `WS /api/jobs/{job_id}/stream` (optional websockets/SSE)
- `POST /api/uninstall`
- `GET /api/system/discovery` (pools, storage, bridges, gpus)

---

## 9.4 Install job state machine (v0)

### States
1. `queued`
2. `validate_request`
3. `validate_manifest`
4. `validate_placement` (pool/storage/bridge)
5. `validate_gpu` (if requested)
6. `allocate_ctid`
7. `create_container`
8. `configure_container` (features, mounts, tags, pool)
9. `attach_gpu` (if requested)
10. `start_container`
11. `wait_for_network` (best-effort; optional)
12. `push_assets` (scripts/templates)
13. `provision` (pct exec)
14. `healthcheck` (optional)
15. `collect_outputs`
16. `completed` or `failed`

### Failure behavior
- Default: leave failed CT stopped and labeled `appstore:failed=1`.
- Provide UI action: “Destroy failed install” (explicit confirm).

### Retry rules
- Network wait: retry with backoff.
- Provisioning: no automatic retry unless manifest says safe-to-retry.

---

## 10. App catalog specification (detailed)

## 10.1 Repo layout
```
catalog/
  apps/
    <app-id>/
      app.yml
      icon.png (optional)
      README.md (optional)
      provision/
        install.sh
        upgrade.sh (optional)
        uninstall.sh (optional)
        healthcheck.sh (optional)
      templates/
        *.tmpl (optional)
```

## 10.2 Manifest schema (v0)

### Top-level
- `id` (string; kebab-case)
- `name` (string)
- `description` (string)
- `version` (semver)
- `categories` (list)
- `tags` (list)
- `homepage` (url optional)
- `license` (optional)
- `maintainers` (optional)

### Placement defaults
- `lxc.ostemplate` (e.g., `debian-12`)
- `lxc.defaults`:
  - `unprivileged` (bool)
  - `cores` (int)
  - `memory_mb` (int)
  - `disk_gb` (int)
  - `bridge` (string)
  - `storage` (string)
  - `features` (list: nesting/keyctl/fuse)
  - `onboot` (bool)

### Inputs
Each input:
- `key` (string)
- `label` (string)
- `type`: `string|number|boolean|secret|select`
- `default` (optional)
- `required` (bool)
- `validation`:
  - regex / min / max / enum
- `help` (optional)

### Provisioning
- `provision.script`: path
- `provision.env`: key-value template map
- `provision.assets`: optional list of files/dirs to push
- `provision.timeout_sec`: default 1800
- `provision.user`: default `root` inside CT (if supported)
- `provision.redact_keys`: list of env keys to redact in logs

### Outputs
List of:
- `key`, `label`, `value` (templated; may reference runtime facts like IP)

### GPU
- `gpu.supported`: `["intel","amd","nvidia"]`
- `gpu.required`: bool
- `gpu.profiles`: list of supported profiles (see below)
- `gpu.notes`: user-facing notes (drivers, packages)

---

## 11. Provisioning execution model (no arbitrary shell)

### 11.1 Script execution contract
- Provision script is executed as:
  - `/bin/bash /opt/appstore/provision/install.sh`
- Inputs are provided via:
  - env vars (non-secret)
  - a config file `/opt/appstore/inputs.json` (secrets optional)
- Scripts must:
  - be non-interactive
  - exit non-zero on failure
  - write important info to stdout (controller captures)

### 11.2 Asset injection
- Controller copies a per-job directory into CT:
  - `/opt/appstore/provision/`
- Files are pushed using `pct push` (preferred) or bind-mount (optional).
- After install:
  - controller may remove `/opt/appstore` from CT unless manifest says keep.

---

## 12. GPU support design (first-class)

> Goal: make GPU attachment safe, predictable, and user-friendly for LXC workloads.

### 12.1 GPU discovery (host)
Controller enumerates:
- `/dev/dri/*` (Intel/AMD iGPU render nodes)
- `/dev/nvidia*` (NVIDIA device nodes)
- `lspci` classification (optional)
- `nvidia-smi` presence (optional)
- map: device → type → friendly name

### 12.2 GPU “profiles”
Profiles define how to attach devices into an LXC safely.

#### Profile: `dri-render` (Intel/AMD/modern iGPU path)
- Bind-mount:
  - `/dev/dri` → `dev/dri`
- Allow devices (cgroup v2):
  - char major `226` (DRM devices)
- Ensure container user is in `video` / `render` group (as needed)

Many guides mount `/dev/dri` and allow DRM device nodes for unprivileged LXCs. citeturn0search8turn0search16turn0search12

#### Profile: `nvidia-basic`
- Bind-mount:
  - `/dev/nvidia0`, `/dev/nvidiactl`, `/dev/nvidia-uvm`, `/dev/nvidia-uvm-tools` (as present)
  - optionally `/dev/dri` as well (some stacks need it)
- Allow device nodes via cgroup v2 for NVIDIA majors (implementation will detect majors dynamically)
- Validate host driver:
  - `nvidia-smi` success on host before attach (best-effort)

Community reports show LXC NVIDIA passthrough commonly relies on binding `/dev/nvidia*` and handling permissions. citeturn0search1turn0search9turn0search5

#### Future profiles (v1+)
- `nvidia-mig` (MIG slices)
- `nvidia-vgpu` (licensed; environment-specific)
- `rocm` (AMD compute stacks)

### 12.3 Attach mechanism (implementation approach)
Because the controller runs on the host, it can:
1. Create CT
2. Write/merge GPU-related lines into `/etc/pve/lxc/<ctid>.conf`
3. Restart CT if required

**Policy:**
- Only attach GPUs to CTs tagged `appstore:managed=1`.
- Refuse if CT is privileged unless explicitly enabled.

### 12.4 PVE version considerations
Newer PVE versions expose passthrough device UI for LXCs (Resources → Add → Passthrough Device) and commonly map devices like `/dev/dri/renderD128`. citeturn0search12turn0search4  
This project will implement attachment independently (config-driven) but should remain compatible with PVE-native config style.

### 12.5 GPU validation steps (preflight)
When GPU requested:
- Verify device nodes exist on host.
- Verify requested profile is compatible with host.
- Verify CT config will include needed mounts and cgroup device rules.
- Optional “smoke test” inside CT:
  - for DRI: run `ls /dev/dri` and optionally `vainfo` if installed (manifest-controlled)
  - for NVIDIA: run `nvidia-smi` in CT if toolkit installed (manifest-controlled)

### 12.6 UI/manifest ergonomics
- Apps declare what they need (`gpu.required`, supported types).
- User sees:
  - “GPU optional/required”
  - “Attach GPU now” toggle
  - “Select GPU device(s)” for multi-GPU systems

---

## 13. Templates and image strategy

v0 supports **script-based provisioning** on the local node.

Additionally, support an optional template pipeline to improve speed and reproducibility.

### 13.1 Base OS templates
- Use Proxmox LXC templates (e.g., Debian 12).
- Controller checks template availability and can prompt to download via `pveam` (optional).

### 13.2 App templates (optional)
For popular apps, the catalog may include a prebuilt “golden rootfs”:
- `templates/<app-id>-<version>.tar.zst`
- Manifest can specify `lxc.ostemplate` as this app template.
This enables future cluster-wide installs without remote `exec`, if desired.

---

## 14. Proxmox API integration details

### 14.1 Auth
- Prefer API tokens (no interactive CSRF/ticket flow needed). citeturn0search2turn0search10turn0search6

### 14.2 Endpoint correctness
Proxmox API requires correct HTTP verbs per endpoint (PUT vs POST), and “501 Not Implemented” often indicates wrong method or reverse-proxy issues. citeturn0search15turn0search11turn0search3  
Implementation must follow API Viewer definitions.

### 14.3 API usage (high level)
- Discovery:
  - pools, storage, bridges, nodes
- Lifecycle:
  - create CT
  - set config
  - start/stop/status
- Note: provisioning inside CT is done via local `pct exec` in v0.

---

## 15. UI design requirements (detailed)

### 15.1 “Apps” page
- Search
- Category filter
- Tag filter
- Sort: featured, popular, updated

### 15.2 App detail page
- icon, name, version
- description, homepage
- resources defaults
- GPU capability banner
- “Install” action

### 15.3 Install wizard
Step 1: placement
- pool (fixed per installation config)
- storage, bridge (defaults; allow override)
Step 2: resources
- cores/memory/disk (within caps)
Step 3: GPU
- none / attach
- choose profile + device(s)
Step 4: app inputs
- dynamic fields from manifest
Step 5: review + install

### 15.4 Job details view
- State timeline
- Live logs (SSE/WebSocket)
- Outputs panel
- Buttons:
  - stop job (best-effort)
  - destroy failed container (danger confirm)

---

## 16. Logging, monitoring, and supportability

- Structured logs to file + optional journald.
- Per-job log stream to UI (redaction applied).
- Debug bundle export:
  - job metadata (no secrets)
  - controller config (redacted)
  - last 200 lines logs

---

## 17. Testing plan

### 17.1 Unit tests
- manifest validation
- template rendering
- argv allowlist validator
- GPU profile builder

### 17.2 Integration tests (runs on CI runner)
- schema validation across catalog
- simulated job state transitions

### 17.3 Hardware-in-the-loop (recommended)
- Proxmox node in lab:
  - create CT
  - attach iGPU
  - verify `/dev/dri/renderD*` in CT
- NVIDIA host:
  - verify `/dev/nvidia*` in CT (when driver/toolkit installed)

---

## 18. v0 scope & milestones (implementation plan)

### Milestone 0 — Repo scaffolding
- controller skeleton + config
- systemd service template
- installer script with TUI prompts

### Milestone 1 — Catalog + UI browse
- git fetch catalog
- validate manifests
- app list + detail UI

### Milestone 2 — Install engine (local node)
- allocate CTID
- create/config/start CT
- push assets + provision via pct exec
- logs + outputs

### Milestone 3 — Security hardening
- pool/tag enforcement
- sudo allowlist
- secret redaction
- audit logs

### Milestone 4 — GPU v0
- GPU discovery
- `dri-render` profile attach
- manifest gating (gpu required/optional)
- UI selection
- basic validation

### Milestone 5 — Obsidian LiveSync as reference app
- Convert existing flow into catalog app
- Provide documented outputs and healthcheck

---

## 19. Acceptance criteria (v0)
- One-liner installs controller service with guided prompts and runs UI on configured port.
- User can browse apps and install one into an LXC.
- Installs are logged with states and final outputs.
- Service refuses to manage containers outside the configured pool/tag boundary.
- GPU apps can request and receive a GPU attachment (at least `/dev/dri` path) and validation confirms device presence inside CT.

---

## 20. Roadmap (post-v0)
- Cluster-wide installs:
  - template-based installs across nodes without remote exec
  - optional worker mode (opt-in, security-reviewed)
- Multi-LXC stacks (bundles)
- OIDC auth
- Signed catalogs / provenance
- App upgrades with migrations
- Rich GPU profiles (MIG/vGPU/ROCm)

---

## Appendix A — Why local provisioning (pct exec) and API-only gap
Proxmox API is excellent for management but “exec inside container” is not consistently available as a supported API primitive; many automation stacks use node-local tooling for that part. This PRD intentionally keeps provisioning local to avoid SSH/agents in v0 and to keep the security model straightforward.

---

## Appendix B — GPU passthrough references
- Community and guide references show typical LXC GPU passthrough patterns: bind-mounting device nodes and allowing DRM devices. citeturn0search8turn0search16turn0search0
- Proxmox UI discussions mention passthrough device selection for LXCs (e.g., `/dev/dri/renderD128`). citeturn0search12turn0search4
- NVIDIA passthrough threads highlight common issues/requirements (device nodes, permissions). citeturn0search1turn0search9

