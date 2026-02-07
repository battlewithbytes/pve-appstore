# Building an App for PVE App Store

This tutorial walks you through creating a container app from scratch. By the end you'll have a working app manifest and install script ready to submit to the catalog.

We'll build a simple static site app called **"My Page"** — it installs Nginx, writes a custom HTML page, and exposes it on a configurable port.

## Prerequisites

- A Proxmox VE host with PVE App Store installed
- Familiarity with Debian/Linux basics (apt, systemd)
- Basic Python knowledge

## Step 1: Create the App Directory

Every app lives in its own directory under `apps/` in the catalog repo:

```
apps/my-page/
  app.yml               # App manifest (required)
  provision/
    install.py          # Install script (required)
  icon.png              # App icon (optional)
  README.md             # Detailed docs (optional)
```

```bash
mkdir -p apps/my-page/provision
```

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
| `packages` | APT packages to install (supports glob: `lib*`) |
| `pip` | pip packages to install in a venv |
| `paths` | Filesystem paths your script can write to (prefix match) |
| `services` | systemd services to enable/start/restart |
| `users` | System users to create |
| `commands` | Binaries your script can run directly |
| `urls` | URLs your script can download from (glob match) |
| `installer_scripts` | Remote scripts allowed to execute (curl\|bash pattern) |
| `apt_repos` | APT repository lines to add |

If your script tries to do something not in its permissions, it fails immediately with `PermissionDeniedError`.

**`outputs`** — Shown to the user after successful installation. Use `{{ip}}` for the container's IP address and `{{input_key}}` for any input value.

**`gpu`** — Set `supported: [intel]`, `[nvidia]`, or `[intel, nvidia]` if your app benefits from GPU passthrough. Use `required: false` unless the app is unusable without a GPU.

## Step 3: Write the Install Script (install.py)

Create `apps/my-page/provision/install.py`:

```python
"""My Page — a simple static website."""

from appstore import BaseApp, run

HTML_TEMPLATE = """\
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
"""

NGINX_CONF = """\
server {
    listen $http_port default_server;
    listen [::]:$http_port default_server;
    root /var/www/html;
    index index.html;
    location / {
        try_files $$uri $$uri/ =404;
    }
}
"""


class MyPageApp(BaseApp):
    def install(self):
        # 1. Install Nginx
        self.apt_install("nginx")

        # 2. Read user inputs
        title = self.inputs.string("title", "My Page")
        message = self.inputs.string("message", "Hello from Proxmox!")
        http_port = self.inputs.string("http_port", "80")

        # 3. Write the HTML page
        self.write_config(
            "/var/www/html/index.html",
            HTML_TEMPLATE,
            title=title,
            message=message,
        )

        # 4. Configure Nginx port (if not default)
        if http_port != "80":
            self.write_config(
                "/etc/nginx/sites-available/default",
                NGINX_CONF,
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

The SDK handles loading inputs, permissions, and calling your `install()` method. You never parse command-line arguments or read config files directly.

### Key Concepts

**Reading inputs** — Use typed accessors on `self.inputs`:
```python
name = self.inputs.string("key", "default")
port = self.inputs.integer("port", 8080)
enabled = self.inputs.boolean("flag", False)
token = self.inputs.secret("api_token")  # redacted in logs
```

**Config templates** — `write_config()` uses Python's `string.Template` syntax:
- `$variable` — substituted with your keyword arguments
- `$$` — literal `$` character (needed for Nginx's `$uri`, etc.)

**Logging** — Use `self.log` for structured output:
```python
self.log.info("Installing dependencies")
self.log.warn("Port conflict detected")
self.log.error("Failed to start service")
self.log.progress(2, 5, "Configuring service")
self.log.output("admin_url", "http://localhost:8080/admin")
```

## Step 4: Understand the Available Helpers

Every helper validates against your `permissions` block before executing.

### Package Management

```python
# APT packages (must be in permissions.packages)
self.apt_install("nginx", "curl", "gnupg")

# pip packages in a virtual environment (must be in permissions.pip)
self.create_venv("/opt/myapp/venv")
self.pip_install("flask", "gunicorn", venv="/opt/myapp/venv")
```

### File Operations

```python
# Write a config file from a template (path must be in permissions.paths)
self.write_config("/etc/myapp/config.yml", TEMPLATE, port=port, host=host)

# Create a directory (path must be in permissions.paths)
self.create_dir("/opt/myapp/data", owner="myapp", mode="0750")

# Change ownership (path must be in permissions.paths)
self.chown("/opt/myapp", "myapp:myapp", recursive=True)

# Download a file (URL must match permissions.urls, dest in permissions.paths)
self.download("https://example.com/release.tar.gz", "/opt/myapp/release.tar.gz")
```

### System Operations

```python
# Enable and start a systemd service (must be in permissions.services)
self.enable_service("myapp")

# Restart a service (must be in permissions.services)
self.restart_service("myapp")

# Create a system user (must be in permissions.users)
self.create_user("myapp", system=True, home="/opt/myapp", shell="/bin/bash")

# Run a command (binary must be in permissions.commands)
self.run_command(["myapp-setup", "--init"])
```

### APT Repository Management

```python
# Add a signing key (URL must match permissions.urls)
self.add_apt_key(
    "https://repo.example.com/gpg.key",
    "/usr/share/keyrings/example-keyring.gpg"
)

# Add a repository (must match permissions.apt_repos)
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

## Step 5: Optional Lifecycle Methods

Beyond `install()`, you can implement additional lifecycle methods:

```python
class MyPageApp(BaseApp):
    def install(self):
        """Required. Runs during initial installation."""
        ...

    def configure(self):
        """Optional. Runs after install for additional setup."""
        ...

    def healthcheck(self) -> bool:
        """Optional. Returns True if the app is healthy."""
        import urllib.request
        try:
            port = self.inputs.string("http_port", "80")
            urllib.request.urlopen(f"http://localhost:{port}", timeout=5)
            return True
        except Exception:
            return False

    def uninstall(self):
        """Optional. Cleanup when the app is removed."""
        ...
```

## Step 6: Test Locally

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

## Step 7: Submit to the Catalog

1. Fork the [pve-appstore-catalog](https://github.com/battlewithbytes/pve-appstore-catalog) repo
2. Add your `apps/my-page/` directory
3. Open a pull request

Your app will be reviewed for:
- Manifest completeness and correct permissions
- Install script uses SDK helpers (no raw `subprocess` calls bypassing permissions)
- Reasonable container defaults (don't request 32GB RAM for a static site)
- Permissions are minimal — only declare what you actually need

## Real-World Examples

### App that uses a remote installer (like Ollama)

```python
class OllamaApp(BaseApp):
    def install(self):
        self.run_installer_script("https://ollama.ai/install.sh")
        # Configure via systemd override
        self.create_dir("/etc/systemd/system/ollama.service.d")
        self.write_config(
            "/etc/systemd/system/ollama.service.d/override.conf",
            OVERRIDE_TEMPLATE,
            host=self.inputs.string("bind_address", "0.0.0.0"),
            port=self.inputs.string("api_port", "11434"),
        )
        self.restart_service("ollama")
```

### App with Python venv (like Home Assistant)

```python
class HomeAssistantApp(BaseApp):
    def install(self):
        self.apt_install("python3", "python3-venv", "python3-pip")
        config_path = self.inputs.string("config_path", "/opt/homeassistant/config")
        self.create_dir(config_path)
        self.create_venv("/opt/homeassistant/venv")
        self.pip_install("homeassistant", venv="/opt/homeassistant/venv")
        self.write_config("/etc/systemd/system/homeassistant.service",
                          SERVICE_TEMPLATE, config_path=config_path)
        self.enable_service("homeassistant")
```

### App with APT repository (like Plex)

```python
class PlexApp(BaseApp):
    def install(self):
        self.apt_install("curl")
        self.add_apt_key(
            "https://downloads.plex.tv/plex-keys/PlexSign.key",
            "/usr/share/keyrings/plex-archive-keyring.gpg",
        )
        self.add_apt_repo(
            "deb [signed-by=/usr/share/keyrings/plex-archive-keyring.gpg] "
            "https://downloads.plex.tv/repo/deb public main",
            "plexmediaserver.list",
        )
        self.apt_install("plexmediaserver")
        self.enable_service("plexmediaserver")
```

## Quick Reference

| Helper | Permission Check | Description |
|--------|-----------------|-------------|
| `apt_install(*pkgs)` | `packages` | Install APT packages |
| `pip_install(*pkgs, venv=)` | `pip` | Install pip packages |
| `create_venv(path)` | `paths` | Create Python venv |
| `write_config(path, tmpl, **kw)` | `paths` | Write templated config file |
| `create_dir(path, owner=, mode=)` | `paths` | Create directory |
| `chown(path, owner, recursive=)` | `paths` | Change ownership |
| `download(url, dest)` | `urls` + `paths` | Download a file |
| `enable_service(name)` | `services` | Enable + start service |
| `restart_service(name)` | `services` | Restart service |
| `create_user(name, ...)` | `users` | Create system user |
| `run_command(cmd)` | `commands` | Run a binary |
| `run_installer_script(url)` | `installer_scripts` | Download + run script |
| `add_apt_key(url, path)` | `urls` + `paths` | Add APT signing key |
| `add_apt_repo(line, file)` | `apt_repos` + `paths` | Add APT repository |
