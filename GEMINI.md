# PVE App Store - Gemini CLI Context

This document provides a comprehensive overview of the `pve-appstore` project, designed to serve as contextual information for the Gemini CLI agent.

## Project Overview

The PVE App Store is an application store for Proxmox VE, enabling one-click deployment of LXC containerized applications. It functions similarly to "Unraid Apps," providing a user-friendly interface for browsing, configuring, and deploying apps from a Git-based catalog.

**Key Features:**

*   **One-click Installs:** Web UI for browsing, configuring, and deploying apps.
*   **Git-based Catalog:** Apps are defined by YAML manifests in a Git repository, ensuring automatic updates.
*   **Sandboxed Provisioning:** Utilizes a Python SDK with allowlist-enforced permissions, limiting app actions to declared packages, file writes, and service enablement.
*   **GPU Passthrough:** Supports Intel QSV and NVIDIA profiles for media transcoding and AI workloads.
*   **Proxmox-native:** Leverages the Proxmox REST API for container lifecycle management and runs as a systemd service.
*   **Unprivileged by Default:** Containers run unprivileged, with minimal `sudo` usage (`pct exec`/`push` only).

**Architecture:**

The system consists of a Go binary (`pve-appstore`) that orchestrates several components:

*   **Web UI (React SPA):** Frontend for user interaction, built with React and TypeScript.
*   **REST API:** Provides endpoints for app catalog management, install jobs, and authentication.
*   **Catalog Module:** Handles Git operations (clone/pull) and manifest validation.
*   **Install Engine:** Manages a job queue and the container provisioning pipeline.
*   **Proxmox API Client:** Interfaces with the Proxmox REST API for container lifecycle and task polling.
*   **Python SDK (embedded):** Executes sandboxed provisioning scripts inside containers, enforcing permission boundaries.

**Security Model:**

The project prioritizes security with a least-privilege design:

*   **Unprivileged Service:** The `pve-appstore` service runs as an unprivileged `appstore` Linux user under `systemd`.
*   **Proxmox REST API:** Primarily used for container lifecycle operations and cluster queries via API tokens, significantly reducing `sudo` reliance.
*   **Strict Sudoers Allowlist:** Only `pct exec`, `pct push`, and `pct set` commands are permitted via `sudo` through a highly restricted `/etc/sudoers.d/pve-appstore` configuration.
*   **Systemd Sandboxing:** The service unit employs directives like `ProtectSystem=strict`, `ProtectHome=yes`, and `PrivateTmp=yes` to isolate the process.
*   **Mount Namespace Escape (`nsenter`):** For `pct` commands that require writing to the host filesystem, `nsenter --mount=/proc/1/ns/mnt` is used to temporarily enter the host's mount namespace, ensuring the main service process remains sandboxed.
*   **GPU Passthrough:** Validates device nodes, applies passthrough via `pct set`, and automatically bind-mounts host NVIDIA libraries into containers to ensure version compatibility.
*   **Container Boundaries:** The service only manages containers within its configured Proxmox pool and specifically those tagged `appstore;managed`.
*   **Secret Handling:** API tokens and sensitive data are never logged and are handled securely.

## Building and Running

### Requirements

*   Proxmox VE 8.x+
*   Single Proxmox node

### Quick Start Installation

To install the PVE App Store on a Proxmox host:

```bash
curl -fsSL https://github.com/battlewithbytes/pve-appstore/releases/latest/download/install.sh | bash
```

This interactive installer will guide through host detection, binary download, TUI setup wizard (Proxmox API token, catalog URL, network bridge, storage), systemd service creation, and web UI startup.

### Development Workflow

The project uses `go.mod` for Go dependencies and `npm` for the React frontend. `Makefile` orchestrates common development tasks.

*   **Install Dependencies:**
    ```bash
    make deps
    # This will download Go modules and install npm packages for the frontend.
    ```
*   **Build Binary:**
    ```bash
    make build
    # Compiles the Go binary `pve-appstore`.
    ```
*   **Run Tests (Go & Python SDK):**
    ```bash
    make test
    # Runs Go tests. For Python SDK tests:
    cd sdk/python && python3 -m pytest tests/
    ```
*   **Build Frontend:**
    ```bash
    make frontend
    # Builds the React frontend located in `web/frontend`.
    ```
*   **Run Development Server:**
    ```bash
    make run-serve
    # Runs the Go binary in serve mode, using `dev-config.yml` and a test catalog.
    ```
*   **Cross-compile Release Binaries:**
    ```bash
    make release
    # Builds production-ready binaries for Linux AMD64 and ARM64.
    ```

## Development Conventions

### Go Backend

The Go backend utilizes `cobra` for CLI commands and is structured with internal packages for specific functionalities (`config`, `catalog`, `server`, `engine`, `proxmox`, `pct`, `installer`, `version`, `ui`).

### React Frontend

The web user interface is a Single Page Application (SPA) built with React and TypeScript, located in `web/frontend`.

### App Development (Catalog Applications)

Applications for the PVE App Store are defined in a separate Git repository (`pve-appstore-catalog`) and consist of:

1.  **App Directory Structure:**
    ```
    apps/my-app/
      app.yml               # App manifest (required)
      provision/
        install.py          # Install script (required)
      icon.png              # App icon (optional)
      README.md             # Detailed docs (optional)
    ```

2.  **`app.yml` Manifest:**
    This YAML file describes the app, including:
    *   `id`, `name`, `description`, `version`, `categories`, `tags`, `homepage`, `license`, `maintainers`.
    *   **`lxc`:** Default LXC container settings (e.g., `ostemplate`, `unprivileged`, `cores`, `memory_mb`, `disk_gb`, `features`, `onboot`).
    *   **`inputs`:** User-configurable parameters (e.g., `string`, `number`, `boolean`, `select`, `secret`) with validation.
    *   **`permissions`:** A critical allowlist defining what the `install.py` script can do (e.g., `packages` to install, `paths` to write to, `services` to manage, `commands` to run, `urls` to download from, `apt_repos`, `pip` packages, `users`, `installer_scripts`). **The Python SDK strictly enforces these permissions.**
    *   **`provisioning`:** Specifies the `install.py` script and `timeout_sec`.
    *   **`outputs`:** Information displayed to the user after installation (e.g., access URLs using `{{ip}}` and input values).
    *   **`gpu`:** Declares supported GPU types and if a GPU is `required`.

3.  **`install.py` Script:**
    A Python script that inherits from `appstore.BaseApp` and implements an `install()` method. It interacts with the container environment exclusively through the `appstore` Python SDK, which provides helper functions for:
    *   Package management (`apt_install`, `pip_install`, `create_venv`)
    *   File operations (`write_config`, `create_dir`, `chown`, `download`)
    *   System operations (`enable_service`, `restart_service`, `create_user`, `run_command`)
    *   APT repository management (`add_apt_key`, `add_apt_repo`)
    *   Running remote installer scripts (`run_installer_script`)
    The SDK handles input reading (`self.inputs.string("key")`) and templating (`self.write_config`).

4.  **Lifecycle Methods:**
    App scripts can optionally implement `configure()`, `healthcheck()`, and `uninstall()` methods in addition to `install()`.

5.  **Local Testing:**
    Developers can test their app manifests by copying them to `testdata/catalog/apps/` and running `make test` and `make run-serve` to verify functionality.

6.  **Submission:**
    Apps are submitted to the `pve-appstore-catalog` repository via pull requests, where they are reviewed for security, minimal permissions, and correct usage of SDK helpers.

---
This `GEMINI.md` file provides a foundational understanding of the `pve-appstore` project for future interactions.