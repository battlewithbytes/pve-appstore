# Dockerfile Import — Design & Limitations

## What It Does

The Dockerfile import feature in Developer Mode analyzes a Dockerfile and generates a starting `app.yml` manifest and `install.py` provisioning script. It translates Dockerfile instructions (`FROM`, `RUN`, `COPY`, `ENV`, `EXPOSE`, `ENTRYPOINT`) into PVE App Store SDK calls.

## What It Does NOT Do

**Dockerfile import is a scaffolding tool, not a magic bullet.** It gets you ~60-80% of the way to a working LXC app, but the generated output will almost always need manual editing. Here's why:

### Docker != LXC

Docker images rely on behaviors that don't exist in LXC containers:

- **Init systems (s6-overlay, supervisord, tini)** — Docker images use these to manage multiple processes. LXC uses systemd or OpenRC natively. The scaffold strips Docker init wrappers but cannot automatically convert s6 service definitions or supervisord configs into native services.
- **Entrypoint scripts** — Many images have complex `entrypoint.sh` scripts that create users, generate configs, set permissions, and start services. These need to be manually translated into SDK calls.
- **Build-time vs runtime** — Dockerfiles mix build-time operations (multi-stage builds, `COPY --from=`) with runtime config. The scaffold can only extract the final stage's `RUN` commands.
- **Layer caching** — Docker layers are additive. The scaffold flattens all `RUN` commands, which may include redundant or conflicting operations.

### What You'll Typically Need to Fix

1. **Service management** — Replace Docker init system commands with `create_service()` or `enable_service()`
2. **Configuration files** — Move inline config strings to template files in `provision/` and use `render_template()` or `deploy_provision_file()`
3. **Missing setup logic** — Add configuration that Docker init scripts (cont-init.d, entrypoint.sh) would normally handle
4. **Input types** — Verify that `inputs.integer()` is used for number fields, `inputs.boolean()` for booleans (not `inputs.string()` for everything)
5. **Package names** — Docker (Alpine/Debian) package names may differ from the target LXC OS template
6. **Permissions** — The generated `permissions:` block is a best-effort guess; review and adjust

### Recommended Workflow

1. Import the Dockerfile to generate the scaffold
2. Read the generated `install.py` — understand what it's trying to do
3. Research the original app's documentation and Docker entrypoint scripts
4. Rewrite the install script using SDK best practices (templates, typed inputs, proper service management)
5. Test iteratively using Deploy in Developer Mode

The SWAG app is a good example: the Dockerfile import produced a script that installed packages but resulted in nginx 404 errors because the s6-overlay init scripts that configure nginx, certbot, and fail2ban never ran. The final working version required a complete rewrite with 9 template files and a custom certbot integration.

## Init System Detection (Future Work)

When importing a Dockerfile, the scaffold generator doesn't yet understand Docker init systems. It strips Docker init commands and falls back to `enable_service()`. This means s6-overlay images like SWAG produce install.py scripts that install packages but never run the s6 init scripts that configure the app.

**Goal:** Detect which init system a Docker image uses, and generate init-system-aware install.py scripts. Separate Go file per handler for clean code. Start with s6-overlay (SWAG) since it's the most complex and already tested.

**Key finding:** `OSProfile.ServiceInit` ("systemd"/"openrc") is already defined in `base_images.yml` but never used in scaffold generation. The Python SDK's `create_service()` and `enable_service()` already handle both systemd and OpenRC.

## Init Systems (by prevalence)

| Init System | Prevalence | Detection Signal | LXC Strategy |
|---|---|---|---|
| **Bare process** | ~30-40% | No ENTRYPOINT or binary ENTRYPOINT, just CMD | Use CMD as exec_start (current behavior) |
| **tini / dumb-init** | ~20-30% | `tini` or `dumb-init` in ENTRYPOINT | Strip wrapper, use wrapped command as exec_start |
| **Custom entrypoint.sh** | ~20-30% | Script path in ENTRYPOINT (`.sh`, `entrypoint`) | TODO comment + use CMD if available |
| **supervisord** | ~5-10% | `supervisord` in CMD/ENTRYPOINT or packages | TODO with instructions to extract `[program:X]` sections |
| **s6-overlay** | ~5-15% | `ENTRYPOINT ["/init"]`, s6-overlay tarballs, LinuxServer base images | Run cont-init.d scripts once, parse service run scripts → native services |

## New Files

| File | Purpose |
|---|---|
| `internal/devmode/init_detect.go` | `InitHandler` interface, constants, detection registry, `isDockerInit()` (moved from scaffold.go) |
| `internal/devmode/init_s6.go` | s6-overlay: detect, run cont-init.d, create services from run scripts |
| `internal/devmode/init_supervisord.go` | supervisord: detect, emit documented TODO |
| `internal/devmode/init_tini.go` | tini/dumb-init: detect, unwrap to real command |
| `internal/devmode/init_bare.go` | Bare process + custom entrypoint (fallback) |
| `internal/devmode/init_detect_test.go` | Detection tests for all init types |
| `internal/devmode/init_s6_test.go` | s6 handler code generation tests |
| `internal/devmode/init_tini_test.go` | Tini unwrap tests |

## Modified Files

| File | Changes |
|---|---|
| `internal/devmode/dockerfile.go` | Add `InitSystem string` field to `DockerfileInfo` |
| `internal/devmode/scaffold.go` | Replace 25-line service section with `DetectInitSystem(df).GenerateServiceSetup(...)`, remove `isDockerInit()` (moved) |
| `internal/devmode/resolve.go` | Propagate `InitSystem` in `MergeDockerfileInfoChain` |

## Phase 1: Interface & Detection (`init_detect.go`)

```go
const (
    InitS6Overlay   = "s6-overlay"
    InitSupervisord = "supervisord"
    InitTini        = "tini"
    InitCustomEntry = "custom-entrypoint"
    InitBare        = "bare"
)

type InitHandler interface {
    Name() string
    Detect(df *DockerfileInfo) bool
    GenerateServiceSetup(sp *strings.Builder, df *DockerfileInfo, p *buildScriptParams, profile OSProfile)
    ExtraPermissions(df *DockerfileInfo) (commands, paths []string)
}

// Priority order: s6 > supervisord > tini > custom-entrypoint > bare (fallback)
func DetectInitSystem(df *DockerfileInfo) InitHandler
```

Move `isDockerInit()` here from scaffold.go (unchanged).

## Phase 2: Bare & Custom Entrypoint Handlers (`init_bare.go`)

**`bareHandler`** — replicates exact current behavior from `scaffold.go:995-1019`:
- `inferImpliedServices()` + `enable_service()` for each
- ExecCmd > EntrypointCmd > StartupCmd priority
- `isDockerInit()` filtering
- `create_service()` or `enable_service()`

**`customEntrypointHandler`** — detects `.sh` / `entrypoint` / `docker-entrypoint` in ENTRYPOINT:
- Emits TODO comment explaining the script needs human review
- Falls back to CMD if available for `create_service()`

This phase must produce **identical output** for all existing test cases (zero behavior change).

## Phase 3: Tini Handler (`init_tini.go`)

Detection: `tini` or `dumb-init` in `EntrypointCmd`.

```go
func unwrapTiniCommand(entrypoint, cmd string) string
// "/usr/bin/tini -- /app/start.sh" → "/app/start.sh"
// "tini -g -- python app.py" → "python app.py"
// "dumb-init -- myapp --port 8080" → "myapp --port 8080"
// If CMD set, append it (Docker ENTRYPOINT+CMD concatenation)
```

Generates `create_service()` with the unwrapped command.

## Phase 4: Supervisord Handler (`init_supervisord.go`)

Detection: `supervisord` in CMD/ENTRYPOINT, `supervisor` in packages, `supervisord.conf` in COPY.

Generates well-documented TODO telling user to extract `[program:X]` sections from the config and convert each to `create_service()`. Falls back to `enable_service()`.

## Phase 5: S6 Handler (`init_s6.go`) — The main event

Detection (any one signal):
1. `ENTRYPOINT == "/init"`
2. `ExecCmd` set + LinuxServer base image
3. `s6-overlay` in RUN commands, ADD instructions, or packages
4. COPY to `/etc/s6-overlay/` or `/etc/cont-init.d/`

Generated install.py code (after the existing COPY/git-clone section):
```python
        # ── s6-overlay → native services ──────────────────────────
        # The source image uses s6-overlay for process management.
        # In LXC, we run the init scripts once and create native services.

        # Run s6 one-time init scripts (user creation, config generation, permissions)
        import glob, os
        for script in sorted(glob.glob("/etc/cont-init.d/*")):
            if os.access(script, os.X_OK):
                self.log.info(f"Running init script: {script}")
                self.run_command(["bash", script], check=False)

        # Enable services detected from base image / packages
        self.enable_service("nginx")     # (from inferImpliedServices)

        # Primary service
        self.create_service("swag",
                            exec_start="/usr/sbin/nginx -g 'daemon off;'")
```

Key behaviors:
- Implied services (nginx from base image) → `enable_service()`
- Primary service from `ExecCmd` (s6 run script) → `create_service()`
- If `ExecCmd` empty → `enable_service()` with TODO comment
- `check=False` on all cont-init.d calls (they may reference Docker-specific paths)

Helper: `isLinuxServerImage(baseImage string) bool`

## Phase 6: Scaffold Refactoring (`scaffold.go`)

Replace lines 995-1019 in `buildInstallScript()` with:
```go
    // Service management — delegate to init system handler
    sp.WriteString("\n")
    handler := DetectInitSystem(df)
    df.InitSystem = handler.Name()
    handler.GenerateServiceSetup(&sp, df, &p, ProfileFor(df.BaseOS))
```

Also update manifest permission generation to call `handler.ExtraPermissions(df)`.

## Phase 7: Propagation

- `dockerfile.go`: Add `InitSystem string` to `DockerfileInfo`
- `resolve.go`: In `MergeDockerfileInfoChain`, propagate `InitSystem` from app layer (last wins)

## Implementation Order

1. `init_detect.go` + `init_bare.go` → refactor `scaffold.go` → run all existing tests (zero behavior change)
2. `init_tini.go` + tests
3. `init_supervisord.go` + tests
4. `init_s6.go` + tests
5. `dockerfile.go` + `resolve.go` field additions
6. Test against real LinuxServer Dockerfiles (SWAG, Plex, Sonarr, Jellyfin)
7. `make deploy` and test SWAG import end-to-end

## Verification

1. `go test ./internal/devmode/` — all tests pass (including new init tests)
2. `go test ./internal/server/` — server tests pass
3. `make build` — compiles
4. Deploy and import SWAG Dockerfile → verify install.py has:
   - `cont-init.d` glob runner
   - `enable_service("nginx")` from base image detection
   - `create_service("swag", exec_start="...")` from s6 run script
   - `check=False` on init scripts
5. Import a tini-based image → verify wrapper is stripped
6. Import a bare CMD image → verify identical output to before

## Test Dockerfiles

| Image | Init System | URL |
|---|---|---|
| SWAG | s6-overlay | `https://raw.githubusercontent.com/linuxserver/docker-swag/master/Dockerfile` |
| Plex | s6-overlay | `https://raw.githubusercontent.com/linuxserver/docker-plex/master/Dockerfile` |
| Sonarr | s6-overlay | `https://raw.githubusercontent.com/linuxserver/docker-sonarr/master/Dockerfile` |
| Jellyfin | s6-overlay | `https://raw.githubusercontent.com/linuxserver/docker-jellyfin/master/Dockerfile` |
