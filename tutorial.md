# Building an App for [PVE App Store](https://github.com/battlewithbytes/pve-appstore)

This tutorial walks you through creating a container app from scratch. By the end you'll have a working app manifest and install script ready to submit to the [catalog](https://github.com/battlewithbytes/pve-appstore-catalog).

We'll build a simple static site app called **"My Page"** — it installs Nginx, writes a custom HTML page, and exposes it on a configurable port.

## Prerequisites

- A Proxmox VE host with [PVE App Store](https://github.com/battlewithbytes/pve-appstore) installed
- Familiarity with Debian/Linux basics (apt, systemd)
- Basic Python knowledge

## Step 1: Create the App Directory

Every app lives in its own directory under `apps/` in the [catalog repo](https://github.com/battlewithbytes/pve-appstore-catalog):

```
apps/my-page/
  app.yml               # App manifest (required)
  provision/
    install.py          # Install script (required)
    index.html          # Template files (recommended)
    default.conf        # Keep config as separate files
  icon.png              # App icon (optional, displayed in the web UI)
  README.md             # Detailed docs (optional)
```

```bash
mkdir -p apps/my-page/provision
```

> **Tip:** You can also use **Developer Mode** in the web UI to create apps interactively — it provides a code editor, validation, and deploy/test workflow without touching the filesystem directly. See [Developer Mode](#developer-mode) below.

### App Icon

Include an `icon.png` in your app directory to give it a logo in the [PVE App Store](https://github.com/battlewithbytes/pve-appstore) web UI. Without one, the UI shows the first letter of the app name as a placeholder.

**Icon guidelines:**
- **Format:** PNG (`icon.png` — exact filename required)
- **Size:** 128x128 pixels recommended (displayed at 40x40 in the app list and 56x56 in the detail view)
- **Style:** Square with rounded corners looks best; transparent backgrounds work well
- **File size:** Keep it under 50KB — icons are served directly by the API

## Step 2: Write the Manifest (app.yml)

The manifest describes your app — metadata, container defaults, user inputs, permissions, and outputs. Create `apps/my-page/app.yml`:

```yaml
id: my-page
name: My Page
description: A simple static website served by Nginx.
overview: |
  My Page deploys Nginx inside a lightweight LXC container and serves
  a customizable static HTML page. Configure the title, message, and
  port during installation.
version: 1.0.0
categories:
  - web
tags:
  - nginx
  - static-site
homepage: https://nginx.org
license: MIT
maintainers:
  - Your Name

lxc:
  ostemplate: debian-12
  defaults:
    unprivileged: true
    cores: 1
    memory_mb: 128
    disk_gb: 1
    features:
      - nesting
    onboot: true

inputs:
  - key: title
    label: Page Title
    type: string
    default: My Page
    required: false
    group: General
    description: The title shown in the browser tab and page header.
    help: Any text you like

  - key: message
    label: Message
    type: string
    default: Hello from Proxmox!
    required: false
    group: General
    description: The body text displayed on the page.

  - key: http_port
    label: HTTP Port
    type: number
    default: 80
    required: false
    group: Network
    description: The port Nginx listens on.
    help: Must be between 1-65535
    validation:
      min: 1
      max: 65535

permissions:
  packages: [nginx]
  paths: ["/var/www/", "/etc/nginx/"]
  services: [nginx]

provisioning:
  script: provision/install.py
  timeout_sec: 120

outputs:
  - key: url
    label: Web Page
    value: "http://{{ip}}:{{http_port}}"

gpu:
  supported: []
  required: false
```

### Manifest Sections Explained

**`id`** — Unique kebab-case identifier. Must match the directory name.

**`lxc.defaults`** — Container sizing. Keep it minimal — users can override these during install. Always prefer `unprivileged: true`.

**`inputs`** — Parameters the user can configure before installation. Supported types:
- `string` — Free text
- `number` — Integer with optional `min`/`max` validation
- `boolean` — True/false toggle
- `select` — Dropdown with `validation.enum` options
- `secret` — Like string but redacted in logs (for tokens, passwords)

**`permissions`** — The security allowlist. Your install script can **only** use resources declared here. The SDK enforces this at runtime. Available permission categories:

| Key | What it allows |
|-----|---------------|
| `packages` | APT/apk packages to install (supports glob: `lib*`) |
| `pip` | pip packages to install in a venv |
| `paths` | Filesystem paths your script can write to (prefix match) |
| `services` | systemd/OpenRC services to enable/start/restart |
| `users` | System users to create |
| `commands` | Binaries your script can run directly |
| `urls` | URLs your script can download from (glob match) |
| `installer_scripts` | Remote scripts allowed to execute (curl\|bash pattern) |
| `apt_repos` | APT repository lines to add |

If your script tries to do something not in its permissions, it fails immediately with `PermissionDeniedError`.

**`outputs`** — Shown to the user after successful installation. Use `{{ip}}` for the container's IP address and `{{input_key}}` for any input value.

**`gpu`** — Set `supported: [intel]`, `[nvidia]`, or `[intel, nvidia]` if your app benefits from GPU passthrough. Use `required: false` unless the app is unusable without a GPU.

## Step 3: Write Template Files

**Keep configuration files as separate template files in `provision/`** — don't embed multi-line strings as constants in your Python code. This makes templates easier to read, edit, and diff.

Create `apps/my-page/provision/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>$title</title>
  <style>
    body {
      font-family: sans-serif;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      margin: 0;
      background: #1a1a2e;
      color: #fff;
    }
    .container { text-align: center; }
    h1 { font-size: 2rem; margin-bottom: 0.5rem; }
    p { color: #aaa; }
  </style>
</head>
<body>
  <div class="container">
    <h1>$title</h1>
    <p>$message</p>
  </div>
</body>
</html>
```

Create `apps/my-page/provision/default.conf`:

```nginx
server {
    listen $http_port default_server;
    listen [::]:$http_port default_server;
    root /var/www/html;
    index index.html;
    location / {
        try_files $$uri $$uri/ =404;
    }
}
```

### Template Syntax

Templates use Python's `string.Template` syntax with some extensions:
- `$variable` or `${variable}` — substituted with keyword arguments
- `$$` — literal `$` character (needed for Nginx's `$uri`, shell variables, etc.)
- `{{#key}} ... {{/key}}` — conditional block (included when key is truthy)
- `{{^key}} ... {{/key}}` — inverted block (included when key is falsy)

## Step 4: Write the Install Script (install.py)

Create `apps/my-page/provision/install.py`:

```python
"""My Page — a simple static website."""

from appstore import BaseApp, run


class MyPageApp(BaseApp):
    def install(self):
        # 1. Install Nginx
        self.pkg_install("nginx")

        # 2. Read user inputs (use typed accessors matching the manifest type)
        title = self.inputs.string("title", "My Page")
        message = self.inputs.string("message", "Hello from Proxmox!")
        http_port = self.inputs.integer("http_port", 80)

        # 3. Write the HTML page from template
        self.render_template("index.html", "/var/www/html/index.html",
            title=title,
            message=message,
        )

        # 4. Configure Nginx port (if not default)
        if http_port != 80:
            self.render_template("default.conf", "/etc/nginx/sites-available/default",
                http_port=http_port,
            )

        # 5. Enable and start Nginx
        self.enable_service("nginx")

        # 6. Log success
        self.log.info("My Page installed successfully")


run(MyPageApp)
```

### How It Works

1. **Import the SDK** — `from appstore import BaseApp, run`
2. **Subclass `BaseApp`** — Implement the `install()` method
3. **Call `run(YourApp)`** — Registers your class with the SDK runner

The [SDK](https://github.com/battlewithbytes/pve-appstore/tree/main/sdk/python/appstore) handles loading inputs, permissions, and calling your `install()` method. You never parse command-line arguments or read config files directly.

### Key Concepts

**Reading inputs** — Use typed accessors on `self.inputs` that match your manifest's `type:` field:

```python
name = self.inputs.string("key", "default")    # type: string
port = self.inputs.integer("port", 8080)        # type: number
enabled = self.inputs.boolean("flag", False)     # type: boolean
token = self.inputs.secret("api_token")          # type: secret (redacted in logs)
```

Using the right accessor matters: `inputs.integer()` returns an `int`, `inputs.boolean()` returns a `bool`. Don't use `inputs.string()` for numbers or booleans — you'll get string comparison bugs (e.g., `"80" != 80`).

**Template files** — Keep config as separate files in `provision/`, not as Python string constants:

```python
# GOOD — template file in provision/config.yml
self.render_template("config.yml", "/etc/myapp/config.yml", port=port)

# GOOD — copy without substitution
self.deploy_provision_file("static.conf", "/etc/myapp/static.conf")

# GOOD — read contents for further processing
content = self.provision_file("snippet.conf")

# BAD — don't embed multi-line configs as string constants
TEMPLATE = """\
port: $port
"""
self.write_config("/etc/myapp/config.yml", TEMPLATE, port=port)
```

`write_config()` still works but is best reserved for short, one-line configs. For anything over ~3 lines, use a template file.

**Logging** — Use `self.log` for structured output:

```python
self.log.info("Installing dependencies")
self.log.warn("Port conflict detected")
self.log.error("Failed to start service")
self.log.progress(2, 5, "Configuring service")
self.log.output("admin_url", "http://localhost:8080/admin")
```

## Step 5: Understand the Available Helpers

Every helper validates against your `permissions` block before executing.

### Package Management

```python
# OS-aware: uses apt on Debian/Ubuntu, apk on Alpine (PREFERRED)
self.pkg_install("nginx", "curl", "gnupg")

# APT-only (use pkg_install instead for cross-OS support)
self.apt_install("nginx", "curl", "gnupg")

# pip packages in a virtual environment (must be in permissions.pip)
self.pip_install("flask", "gunicorn", venv="/opt/myapp/venv")

# Create a venv explicitly (pip_install auto-creates one at /opt/venv)
self.create_venv("/opt/myapp/venv")
```

### File & Template Operations

```python
# Render a template file from provision/ with variable substitution
self.render_template("config.yml", "/etc/myapp/config.yml", port=port, host=host)

# Copy a file from provision/ without substitution
self.deploy_provision_file("binary.conf", "/etc/myapp/binary.conf", mode="0644")

# Read a provision file as a string (for composing configs)
content = self.provision_file("snippet.conf")

# Write a config from an inline template string (short configs only)
self.write_config("/etc/myapp/port.conf", "listen $port\n", port=port)

# Write a KEY=VALUE environment file
self.write_env_file("/etc/myapp/env", {"PORT": "8080", "HOST": "0.0.0.0"})

# Create a directory (path must be in permissions.paths)
self.create_dir("/opt/myapp/data", owner="myapp", mode="0750")

# Change ownership (path must be in permissions.paths)
self.chown("/opt/myapp", "myapp:myapp", recursive=True)

# Download a file (URL must match permissions.urls, dest in permissions.paths)
self.download("https://example.com/release.tar.gz", "/opt/myapp/release.tar.gz")
```

### Service Management

```python
# Enable and start an existing service (installed by a package)
self.enable_service("nginx")

# Restart a running service
self.restart_service("nginx")

# Create a new service from scratch (systemd on Debian, OpenRC on Alpine)
self.create_service("myapp",
    exec_start="/opt/myapp/bin/server --port 8080",
    description="My Application Server",
    user="myapp",
    working_directory="/opt/myapp",
    environment={"PORT": "8080"},
    environment_file="/etc/myapp/env",
    restart="on-failure",
    restart_sec=10,
)
```

`create_service()` generates the unit file, enables, and starts it in one call. It handles both systemd and OpenRC automatically.

### System Operations

```python
# Create a system user (must be in permissions.users)
self.create_user("myapp", system=True, home="/opt/myapp", shell="/bin/bash")

# Run a command (binary must be in permissions.commands)
self.run_command(["myapp-setup", "--init"])

# Run a command that might fail (non-fatal)
self.run_command(["optional-step"], check=False)

# Wait for a service to come up
if self.wait_for_http("http://localhost:8080", timeout=60, interval=3):
    self.log.info("Service is ready")
```

### APT Repository Management

```python
# High-level: add a repo with signing key in one call (PREFERRED)
self.add_apt_repository(
    "https://downloads.plex.tv/repo/deb",
    key_url="https://downloads.plex.tv/plex-keys/PlexSign.key",
    name="plexmediaserver",
    suite="public",
)

# Low-level: add key and repo separately
self.add_apt_key(
    "https://repo.example.com/gpg.key",
    "/usr/share/keyrings/example-keyring.gpg"
)
self.add_apt_repo(
    "deb [signed-by=/usr/share/keyrings/example-keyring.gpg] https://repo.example.com/deb stable main",
    "example.list"
)
```

### Running Remote Installer Scripts

Some projects provide their own installer (like Ollama, Jellyfin). Use `run_installer_script()` instead of piping curl to bash yourself:

```python
# URL must be in permissions.installer_scripts
self.run_installer_script("https://ollama.ai/install.sh")
```

### Advanced Helpers

```python
# Deploy a status page (CCO-themed monitoring dashboard)
self.status_page(
    port=8081,
    title="My App",
    api_url="http://localhost:8080/api/status",
    fields={"status": "Status", "uptime": "Uptime", "version": "Version"},
)

# Download a binary from a Docker/OCI image (no Docker needed)
self.pull_oci_binary("qmcgaw/gluetun", "/opt/gluetun/gluetun", tag="latest")

# Apply sysctl settings persistently
self.sysctl({"net.ipv4.ip_forward": 1})

# Disable IPv6
self.disable_ipv6()
```

## Step 6: Optional Lifecycle Methods

Beyond `install()`, you can implement additional lifecycle methods:

```python
class MyPageApp(BaseApp):
    def install(self):
        """Required. Runs during initial installation."""
        self.configure()  # Share config logic with configure()

    def configure(self):
        """Optional. Runs for in-place reconfiguration with updated inputs.

        Tip: Put config-writing and service-restart logic here, then call
        self.configure() at the end of install() so the logic is shared.
        """
        port = self.inputs.integer("http_port", 80)
        self.render_template("default.conf", "/etc/nginx/sites-available/default",
            http_port=port)
        self.restart_service("nginx")

    def healthcheck(self) -> bool:
        """Optional. Returns True if the app is healthy."""
        port = self.inputs.integer("http_port", 80)
        return self.wait_for_http(f"http://localhost:{port}", timeout=10)

    def uninstall(self):
        """Optional. Cleanup when the app is removed."""
        ...
```

## Step 7: Test Locally

Before submitting, test your manifest parses correctly:

1. Copy your app directory into the testdata catalog:
   ```bash
   cp -r apps/my-page /path/to/appstore/testdata/catalog/apps/
   ```

2. Run the Go tests to verify manifest validation:
   ```bash
   make test
   ```

3. Start the dev server and verify your app shows up:
   ```bash
   make run-serve
   # Open the web UI and search for "my-page"
   ```

## Step 8: Submit to the Catalog

1. Fork the [pve-appstore-catalog](https://github.com/battlewithbytes/pve-appstore-catalog) repo
2. Add your `apps/my-page/` directory (with `app.yml`, `provision/install.py`, template files, and optionally `icon.png`)
3. Open a pull request

Your app will be reviewed for:
- Manifest completeness and correct permissions
- Install script uses SDK helpers (no raw `subprocess` calls bypassing permissions)
- Template files for configs instead of inline string constants
- Typed input accessors: `inputs.integer()` for `type: number`, `inputs.boolean()` for `type: boolean`
- Reasonable container defaults (don't request 32GB RAM for a static site)
- Permissions are minimal — only declare what you actually need
- Icon included (recommended but not required)

## Developer Mode

Instead of creating files manually, you can use **Developer Mode** in the web UI (Settings > Developer Mode):

1. **Create** — Pick a starter template or import from a Dockerfile/Unraid XML
2. **Edit** — Code editor with SDK autocompletions for the manifest and install script
3. **Validate** — One-click manifest + script validation
4. **Deploy** — Merge into the running catalog for testing
5. **Export** — Download as a zip ready to submit as a PR

### Dockerfile Import

Developer Mode can import a Dockerfile and generate a starting `app.yml` + `install.py`. This is a **scaffolding tool, not a magic bullet** — it gets you ~60-80% of the way but the output almost always needs manual editing. Docker images rely on init systems (s6-overlay, supervisord), complex entrypoint scripts, and layered builds that don't translate directly to LXC.

**Recommended workflow:**
1. Import the Dockerfile to generate the scaffold
2. Read the generated `install.py` and understand what it's trying to do
3. Research the original app's docs and Docker entrypoint scripts
4. Rewrite the install script using SDK best practices (template files, typed inputs, proper service management)
5. Test iteratively using Deploy

## Real-World Examples

### App that uses a remote installer (Ollama)

```python
class OllamaApp(BaseApp):
    def install(self):
        api_port = self.inputs.integer("api_port", 11434)
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        num_ctx = self.inputs.integer("num_ctx", 2048)

        self.run_installer_script("https://ollama.ai/install.sh")

        # Build config from template files
        override = self.provision_file("systemd-override.conf")
        self.create_dir("/etc/systemd/system/ollama.service.d")
        self.write_config(
            "/etc/systemd/system/ollama.service.d/override.conf",
            override,
            bind_address=bind_address,
            api_port=api_port,
            num_ctx=num_ctx,
        )
        self.restart_service("ollama")
```

### App with Python venv (Home Assistant)

```python
class HomeAssistantApp(BaseApp):
    def install(self):
        http_port = self.inputs.integer("http_port", 8123)
        timezone = self.inputs.string("timezone", "America/New_York")
        config_path = self.inputs.string("config_path", "/opt/homeassistant/config")

        self.pkg_install("python3", "python3-venv", "python3-pip",
                         "libffi-dev", "libssl-dev")
        self.create_user("homeassistant", system=True, home="/opt/homeassistant")
        self.create_dir(config_path)
        self.create_venv("/opt/homeassistant/venv")
        self.pip_install("homeassistant", venv="/opt/homeassistant/venv")

        # Config from template file — not inline string
        self.render_template("configuration.yaml", f"{config_path}/configuration.yaml",
            timezone=timezone, http_port=http_port)

        self.create_service("homeassistant",
            exec_start=f"/opt/homeassistant/venv/bin/hass -c {config_path}",
            description="Home Assistant Core",
            user="homeassistant",
            working_directory="/opt/homeassistant",
        )
```

### App with APT repository (Plex)

```python
class PlexApp(BaseApp):
    def install(self):
        http_port = self.inputs.integer("http_port", 32400)
        friendly_name = self.inputs.string("friendly_name", "Proxmox Plex")

        self.add_apt_repository(
            "https://downloads.plex.tv/repo/deb",
            key_url="https://downloads.plex.tv/plex-keys/PlexSign.key",
            name="plexmediaserver",
            suite="public",
        )
        self.apt_install("plexmediaserver")

        # Write preferences from template file
        prefs_dir = "/var/lib/plexmediaserver/Library/Application Support/Plex Media Server"
        self.create_dir(prefs_dir)
        self.render_template("Preferences.xml", f"{prefs_dir}/Preferences.xml",
            friendly_name=friendly_name, http_port=http_port)
        self.chown("/var/lib/plexmediaserver", "plex:plex", recursive=True)
        self.enable_service("plexmediaserver")
```

### Alpine-based app with OCI binary (Gluetun)

```python
class GluetunApp(BaseApp):
    def install(self):
        vpn_type = self.inputs.string("vpn_type", "openvpn")
        httpproxy = self.inputs.boolean("httpproxy", False)
        httpproxy_port = self.inputs.integer("httpproxy_port", 8888)

        self.pkg_install("iptables", "ip6tables", "ca-certificates", "unbound")
        self.pull_oci_binary("qmcgaw/gluetun", "/opt/gluetun/gluetun", tag="latest")

        self.render_template("start.sh", "/opt/gluetun/start.sh",
            vpn_type=vpn_type)
        self.deploy_provision_file("start.sh", "/opt/gluetun/start.sh", mode="0755")

        self.create_service("gluetun",
            exec_start="/opt/gluetun/start.sh",
            capabilities=["NET_ADMIN"],
        )
```

## Quick Reference

| Helper | Permission Check | Description |
|--------|-----------------|-------------|
| `pkg_install(*pkgs)` | `packages` | OS-aware package install (apt/apk) |
| `apt_install(*pkgs)` | `packages` | APT-only package install |
| `pip_install(*pkgs, venv=)` | `pip` | Install pip packages in a venv |
| `create_venv(path)` | `paths` | Create Python venv |
| `render_template(name, dest, **kw)` | `paths` | Render a template file from `provision/` |
| `deploy_provision_file(name, dest)` | `paths` | Copy a provision file without substitution |
| `provision_file(name)` | — | Read a provision file as a string |
| `write_config(path, tmpl, **kw)` | `paths` | Write from inline template string |
| `write_env_file(path, env_dict)` | `paths` | Write KEY=VALUE environment file |
| `create_dir(path, owner=, mode=)` | `paths` | Create directory |
| `chown(path, owner, recursive=)` | `paths` | Change ownership |
| `download(url, dest)` | `urls` + `paths` | Download a file |
| `create_service(name, exec_start=, ...)` | `services` | Create + enable + start a new service |
| `enable_service(name)` | `services` | Enable + start an existing service |
| `restart_service(name)` | `services` | Restart a service |
| `create_user(name, ...)` | `users` | Create system user |
| `run_command(cmd)` | `commands` | Run a binary |
| `run_installer_script(url)` | `installer_scripts` | Download + run script |
| `add_apt_repository(url, key_url=, ...)` | `urls` + `apt_repos` | Add APT repo + key (high-level) |
| `add_apt_key(url, path)` | `urls` + `paths` | Add APT signing key |
| `add_apt_repo(line, file)` | `apt_repos` + `paths` | Add APT repository |
| `wait_for_http(url, timeout=)` | `urls` | Poll until HTTP 200 |
| `status_page(port, title, ...)` | `services` + `paths` | Deploy status page dashboard |
| `pull_oci_binary(image, dest)` | `urls` + `paths` | Download binary from Docker image |
| `sysctl(settings)` | `paths` | Apply sysctl settings |
| `disable_ipv6()` | `paths` | Disable IPv6 via sysctl |
