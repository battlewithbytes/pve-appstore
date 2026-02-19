"""BaseApp abstract class for PVE App Store provisioning.

All app install scripts subclass BaseApp and implement the install() method.
Helper methods enforce the app's declared permission allowlist before executing
any system operations.
"""

import base64
import hashlib
import json
import os
import secrets
import shutil
import string
import subprocess
import tempfile
import time
import urllib.request
from abc import ABC, abstractmethod

from appstore.inputs import AppInputs
from appstore.logging import AppLogger
from appstore.osdetect import detect_os
from appstore.permissions import AppPermissions
from appstore.templates import render


class BaseApp(ABC):
    """Abstract base class for app provisioning scripts."""

    def __init__(self, inputs: AppInputs, permissions: AppPermissions):
        self.inputs = inputs
        self.permissions = permissions
        self.log = AppLogger()
        self._os = detect_os()

        if self._os == "alpine":
            from appstore import platform_alpine as _plat
        else:
            from appstore import platform_debian as _plat
        self._platform = _plat

    # --- Lifecycle methods (subclasses implement these) ---

    @abstractmethod
    def install(self) -> None:
        """Install the application. Must be implemented by subclasses."""
        ...

    def configure(self) -> None:
        """Optional post-install configuration."""
        pass

    def healthcheck(self) -> bool:
        """Optional health check. Return True if healthy."""
        return True

    def uninstall(self) -> None:
        """Optional cleanup for uninstall."""
        pass

    # @sdk-group: Package Management

    def apt_install(self, *packages: str) -> None:
        """Install apt packages (deprecated — use pkg_install for OS-aware installs). Each package must be in permissions.packages."""
        for pkg in packages:
            self.permissions.check_package(pkg)

        self.log.info(f"Installing apt packages: {', '.join(packages)}")
        self._run(["apt-get", "update", "-qq"])
        self._run(["apt-get", "install", "-y", "-qq"] + list(packages))

    def pkg_install(self, *packages: str) -> None:
        """OS-aware package install. Uses apt on Debian, apk on Alpine.

        Each package must be in permissions.packages.
        """
        for pkg in packages:
            self.permissions.check_package(pkg)
        self._platform.pkg_install(self, list(packages))

    def pip_install(self, *packages: str, venv: str = None) -> None:
        """Install pip packages in a virtual environment.

        Uses /opt/venv by default to comply with PEP 668 (externally-managed
        Python environments on modern distros). Pass venv= to override.
        """
        for pkg in packages:
            self.permissions.check_pip_package(pkg)

        venv_path = venv or self._default_venv
        if not os.path.isfile(f"{venv_path}/bin/pip"):
            self.log.info(f"Creating venv: {venv_path}")
            self._run(["python3", "-m", "venv", venv_path])

        pip_bin = f"{venv_path}/bin/pip"
        self.log.info(f"Installing pip packages: {', '.join(packages)}")
        self._run([pip_bin, "install", "--progress-bar", "off"] + list(packages))

    _default_venv = "/opt/venv"

    def create_venv(self, path: str) -> None:
        """Create a Python virtual environment."""
        self.permissions.check_path(path)
        self.log.info(f"Creating venv: {path}")
        self._run(["python3", "-m", "venv", path])
        # Upgrade pip in the new venv
        self._run([f"{path}/bin/pip", "install", "--progress-bar", "off", "-U", "pip"])

    def enable_repo(self, repo_name: str) -> None:
        """Enable a named repository. Alpine: uncomments in /etc/apk/repositories.
        Debian: no-op (use add_apt_repository instead)."""
        self._platform.enable_repo(self, repo_name)

    def add_apt_key(self, url: str, keyring_path: str) -> None:
        """Add an APT signing key from a URL.

        If keyring_path ends with .gpg, the key is dearmored (binary format).
        If keyring_path ends with .asc or anything else, the key is saved as-is.
        Modern apt handles both formats with signed-by=.
        """
        if not hasattr(self._platform, "add_apt_key"):
            raise RuntimeError("apt methods are only available on Debian/Ubuntu")
        self.permissions.check_url(url)
        self.permissions.check_path(keyring_path)
        self._platform.add_apt_key(self, url, keyring_path)

    def add_apt_repo(self, repo_line: str, filename: str) -> None:
        """Add an APT repository source file."""
        if not hasattr(self._platform, "add_apt_repo"):
            raise RuntimeError("apt methods are only available on Debian/Ubuntu")
        self.permissions.check_apt_repo(repo_line)
        self._platform.add_apt_repo(self, repo_line, filename)

    def add_apt_repository(self, repo_url: str, key_url: str, name: str = "",
                           suite: str = "", components: str = "main") -> None:
        """Add an APT repository with its signing key in one call.

        This is the recommended high-level method. It:
        1. Downloads and installs the GPG signing key
        2. Detects the distro codename (e.g. "noble") if suite is not given
        3. Writes the sources list entry with signed-by

        Args:
            repo_url: Base URL of the repository (e.g. "https://downloads.plex.tv/repo/deb").
            key_url: URL to the GPG signing key.
            name: Short name for the repo (used for filenames). Auto-derived from URL if blank.
            suite: Distribution suite/codename (e.g. "noble", "stable", "public").
                   If blank, auto-detected from /etc/os-release VERSION_CODENAME.
            components: Space-separated components (default: "main").
        """
        if not hasattr(self._platform, "add_apt_repository"):
            raise RuntimeError("apt methods are only available on Debian/Ubuntu")
        self.permissions.check_url(repo_url)
        self.permissions.check_url(key_url)
        self._platform.add_apt_repository(self, repo_url, key_url, name=name,
                                          suite=suite, components=components)

    def pull_oci_binary(self, image: str, dest: str, tag: str = "latest") -> None:
        """Download a binary from a Docker/OCI image without Docker.

        Args:
            image: Docker image name (e.g., "qmcgaw/gluetun").
            dest: Destination path for the extracted binary.
            tag: Image tag (default "latest").
        """
        self.permissions.check_url("https://auth.docker.io/*")
        self.permissions.check_url("https://registry-1.docker.io/*")
        self.permissions.check_path(dest)

        from appstore.oci import OCIClient
        client = OCIClient(log=self.log)
        client.pull_binary(image, dest, tag=tag)

        # Post-extraction validation: check for missing shared libraries
        try:
            result = subprocess.run(
                ["ldd", dest], capture_output=True, text=True
            )
            if result.returncode == 0:
                missing = [
                    line.strip() for line in result.stdout.splitlines()
                    if "not found" in line
                ]
                if missing:
                    self.log.warn(
                        f"Binary at {dest} has missing shared libraries:\n"
                        + "\n".join(f"  {m}" for m in missing)
                    )
        except FileNotFoundError:
            pass  # ldd not available

    # @sdk-group: File Operations

    def write_config(self, path: str, template_str: str, **kwargs) -> str:
        """Write a config file using template substitution.

        Pass the template as an inline string (use render_template to read
        from a file instead).  Supports $variable substitution and
        {{#key}}...{{/key}} / {{^key}}...{{/key}} conditional blocks.

        Args:
            path: Destination path (must be under an allowed path prefix).
            template_str: Template string using $variable syntax and
                          {{#key}}...{{/key}} conditional blocks.
            **kwargs: Variables to substitute.

        Returns:
            The rendered content.
        """
        self.permissions.check_path(path)

        content = render(template_str, **kwargs)
        os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
        with open(path, "w") as f:
            f.write(content)
        self.log.info(f"Wrote config: {path}")
        return content

    def render_template(self, template_name: str, dest_path: str, **kwargs) -> str:
        """Read a template file from the app's provision directory and write it to dest_path.

        Template files support:
          - $variable / ${variable} — value substitution (Python string.Template)
          - {{#key}} ... {{/key}} — conditional block (included when key is truthy)
          - {{^key}} ... {{/key}} — inverted block (included when key is falsy)
          - $$ — literal dollar sign escape

        Args:
            template_name: Filename relative to the provision directory
                           (e.g. "gitlab.rb.tmpl").
            dest_path: Where to write the rendered output.
            **kwargs: Template variables.

        Returns:
            The rendered content.
        """
        # Templates are pushed to /opt/appstore/provision/ alongside install.py
        tmpl_path = os.path.join("/opt/appstore/provision", template_name)
        with open(tmpl_path) as f:
            template_str = f.read()
        return self.write_config(dest_path, template_str, **kwargs)

    def provision_file(self, name: str) -> str:
        """Read a file from the provision directory and return its contents.

        Use this to keep templates, configs, and scripts as real files in the
        provision/ directory instead of embedding them as string constants.

        Args:
            name: Filename (e.g., "server.py", "config.ini").

        Returns:
            File contents as a string.
        """
        path = os.path.join(self._provision_dir(), name)
        with open(path) as f:
            return f.read()

    def deploy_provision_file(self, name: str, dest: str, mode: str = None) -> None:
        """Copy a file from the provision directory to a destination path.

        Args:
            name: Filename in the provision directory.
            dest: Destination path in the container.
            mode: Optional file permissions (octal string, e.g. "0755").
        """
        self.permissions.check_path(dest)
        src = os.path.join(self._provision_dir(), name)
        os.makedirs(os.path.dirname(dest) or ".", exist_ok=True)
        shutil.copy2(src, dest)
        if mode:
            os.chmod(dest, int(mode, 8))
        self.log.info(f"Deployed {name} -> {dest}")

    def write_env_file(self, path: str, env_dict: dict, mode: str = "0644") -> None:
        """Write a KEY=VALUE environment file, skipping None/empty values.

        Args:
            path: Destination path.
            env_dict: Dict of environment variables. Keys with None or empty
                     string values are skipped.
            mode: File permissions (octal string).
        """
        self.permissions.check_path(path)
        lines = []
        for key, value in env_dict.items():
            if value is None or value == "":
                continue
            lines.append(f"{key}={value}")
        content = "\n".join(lines) + "\n" if lines else ""
        os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
        with open(path, "w") as f:
            f.write(content)
        os.chmod(path, int(mode, 8))
        self.log.info(f"Wrote env file: {path} ({len(lines)} vars)")

    def create_dir(self, path: str, owner: str = None, mode: str = "0755") -> None:
        """Create a directory with optional ownership."""
        self.permissions.check_path(path)
        os.makedirs(path, exist_ok=True)
        os.chmod(path, int(mode, 8))
        if owner:
            self._run(["chown", owner, path])
        self.log.info(f"Created directory: {path}")

    def chown(self, path: str, owner: str, recursive: bool = False) -> None:
        """Change file/directory ownership."""
        self.permissions.check_path(path)
        cmd = ["chown"]
        if recursive:
            cmd.append("-R")
        cmd.extend([owner, path])
        self._run(cmd)

    def download(self, url: str, dest: str) -> None:
        """Download a URL to a file."""
        self.permissions.check_url(url)
        self.permissions.check_path(dest)
        self.log.info(f"Downloading {url} -> {dest}")
        os.makedirs(os.path.dirname(dest) or ".", exist_ok=True)
        self._run(["curl", "-fsSL", "-o", dest, url])

    # @sdk-group: Service Management

    def create_service(
        self,
        name: str,
        exec_start: str,
        description: str = None,
        after: str = "network-online.target",
        user: str = None,
        working_directory: str = None,
        environment: dict = None,
        environment_file: str = None,
        restart: str = "always",
        restart_sec: int = 5,
        type: str = "simple",
        capabilities: list = None,
        extra_unit: str = None,
        extra_service: str = None,
    ) -> None:
        """Create, enable, and start a service (systemd on Debian, OpenRC on Alpine).

        Args:
            name: Service name.
            exec_start: Command to run.
            description: Service description.
            after: Dependency (systemd After= / OpenRC need).
            user: User to run as (None = root).
            working_directory: WorkingDirectory.
            environment: Dict of KEY=VALUE env vars.
            environment_file: Path to env file.
            restart: Restart policy.
            restart_sec: Restart delay in seconds (systemd only).
            type: Service type (systemd only).
            capabilities: Linux capabilities list.
            extra_unit: Extra lines for [Unit] section (systemd only).
            extra_service: Extra lines for [Service] section.
        """
        self.permissions.check_service(name)
        self._platform.create_service(
            self, name, exec_start, description=description,
            after=after, user=user, working_directory=working_directory,
            environment=environment, environment_file=environment_file,
            restart=restart, restart_sec=restart_sec, type=type,
            capabilities=capabilities, extra_unit=extra_unit,
            extra_service=extra_service,
        )

    def enable_service(self, name: str) -> None:
        """Enable and start a service (systemd on Debian, OpenRC on Alpine)."""
        self.permissions.check_service(name)
        self.log.info(f"Enabling service: {name}")
        self._platform.enable_service(self, name)

    def restart_service(self, name: str) -> None:
        """Restart a service (systemd on Debian, OpenRC on Alpine)."""
        self.permissions.check_service(name)
        self.log.info(f"Restarting service: {name}")
        self._platform.restart_service(self, name)

    # @sdk-group: User Management

    def create_user(
        self,
        name: str,
        system: bool = True,
        home: str = None,
        shell: str = "/bin/false",
    ) -> None:
        """Create a system user. Uses useradd on Debian, adduser on Alpine."""
        self.permissions.check_user(name)
        self.log.info(f"Creating user: {name}")
        self._platform.create_user(self, name, system=system, home=home, shell=shell)

    # @sdk-group: Commands & System

    def run_command(self, cmd, check: bool = True, input_text: str = None,
                    cwd: str = None, env: dict = None) -> subprocess.CompletedProcess:
        """Run a command. The binary (cmd[0]) must be in permissions.commands.

        cmd can be a list like ["git", "clone", "..."] or a string like "git clone ...".
        Strings are automatically split with shlex.split().

        If input_text is provided, it is written to the process's stdin.
        If cwd is provided, the command runs in that directory.
        If env is provided, the key-value pairs are merged into the environment.
        """
        if isinstance(cmd, str):
            import shlex
            cmd = shlex.split(cmd)
        if not cmd:
            raise ValueError("empty command")
        self.permissions.check_command(cmd[0])
        return self._run(cmd, check=check, input_text=input_text, cwd=cwd, env=env)

    def run_shell(self, cmd: str, check: bool = True, cwd: str = None,
                  env: dict = None) -> subprocess.CompletedProcess:
        """Run a shell command through bash. Use for pipes, redirects, and compound commands.

        The first command word must be in permissions.commands.
        Supports shell features: pipes (|), redirects (>), compound (&&), subshells.

        If cwd is provided, the command runs in that directory.
        If env is provided, the key-value pairs are merged into the environment.
        """
        if not cmd or not cmd.strip():
            raise ValueError("empty shell command")
        import shlex
        first_word = shlex.split(cmd)[0] if cmd.strip() else ""
        self.permissions.check_command(first_word)
        return self._run(["bash", "-c", cmd], check=check, cwd=cwd, env=env)

    def run_installer_script(self, url: str) -> None:
        """Download and run a remote installer script."""
        self.permissions.check_installer_script(url)
        self.log.info(f"Running installer script: {url}")
        with tempfile.NamedTemporaryFile(mode="w", suffix=".sh", delete=False) as f:
            tmp_path = f.name
        try:
            self._run(["curl", "-fsSL", "-o", tmp_path, url])
            os.chmod(tmp_path, 0o755)
            self._run(["bash", tmp_path])
        finally:
            os.unlink(tmp_path)

    def sysctl(self, settings: dict) -> None:
        """Apply sysctl settings persistently.

        Args:
            settings: Dict of sysctl key-value pairs.
        """
        conf_path = "/etc/sysctl.d/99-appstore.conf"
        self.permissions.check_path(conf_path)

        lines = [f"{key} = {value}" for key, value in settings.items()]
        content = "\n".join(lines) + "\n"

        os.makedirs(os.path.dirname(conf_path), exist_ok=True)
        with open(conf_path, "w") as f:
            f.write(content)
        self.log.info(f"Wrote sysctl config: {conf_path}")
        self._run(["sysctl", "--system"], check=False)

    def disable_ipv6(self) -> None:
        """Disable IPv6 system-wide via sysctl."""
        self.log.info("Disabling IPv6...")
        self.sysctl({
            "net.ipv6.conf.all.disable_ipv6": 1,
            "net.ipv6.conf.default.disable_ipv6": 1,
        })

    def wait_for_http(self, url: str, timeout: int = 60, interval: int = 3) -> bool:
        """Poll an HTTP endpoint until it responds 200.

        Args:
            url: URL to poll.
            timeout: Max seconds to wait.
            interval: Seconds between attempts.

        Returns:
            True if a 200 response was received, False on timeout.
        """
        self.permissions.check_url(url)
        self.log.info(f"Waiting for HTTP 200 at {url} (timeout={timeout}s)...")
        attempts = max(1, timeout // interval)
        for i in range(attempts):
            try:
                resp = urllib.request.urlopen(url, timeout=interval)
                if resp.status == 200:
                    self.log.info(f"HTTP 200 received from {url}")
                    return True
            except Exception:
                pass
            if i < attempts - 1:
                time.sleep(interval)
        self.log.warn(f"Timeout waiting for HTTP 200 at {url}")
        return False

    # @sdk-group: Password & Crypto

    def random_password(self, length: int = 16) -> str:
        """Generate a cryptographically secure random password.

        Args:
            length: Password length (default 16, minimum 8).

        Returns:
            Random password string containing letters, digits, and punctuation.
        """
        length = max(length, 8)
        alphabet = string.ascii_letters + string.digits + "!@#$%&*-_=+"
        return "".join(secrets.choice(alphabet) for _ in range(length))

    def pbkdf2_hash(
        self,
        password: str,
        algo: str = "sha512",
        iterations: int = 100000,
        salt_bytes: int = 16,
    ) -> dict:
        """Hash a password using PBKDF2 and return salt + hash as base64.

        Args:
            password: The plaintext password to hash.
            algo: Hash algorithm (default "sha512").
            iterations: Number of PBKDF2 iterations (default 100000).
            salt_bytes: Length of random salt in bytes (default 16).

        Returns:
            Dict with keys "salt" (base64), "hash" (base64), "algo", "iterations".
        """
        salt = os.urandom(salt_bytes)
        dk = hashlib.pbkdf2_hmac(algo, password.encode(), salt, iterations)
        return {
            "salt": base64.b64encode(salt).decode(),
            "hash": base64.b64encode(dk).decode(),
            "algo": algo,
            "iterations": iterations,
        }

    # @sdk-group: Advanced

    def status_page(
        self,
        port: int,
        title: str,
        api_url: str,
        fields: dict,
        bind_lan_only: bool = True,
    ) -> None:
        """Deploy a self-contained status page server with the CCO dark theme.

        Args:
            port: Port for the status page HTTP server.
            title: Page title.
            api_url: URL to poll for status data (JSON).
            fields: Dict mapping API response keys to display labels.
                   First field is shown prominently, rest in a grid.
            bind_lan_only: If True, bind to LAN IP only (not 0.0.0.0).
        """
        self.permissions.check_url(api_url)

        # Determine service name from the app context
        svc_name = title.lower().replace(" ", "-")
        status_svc = f"{svc_name}-status"
        self.permissions.check_service(status_svc)

        # Deploy directory
        deploy_dir = f"/etc/{status_svc}"
        self.permissions.check_path(deploy_dir)
        os.makedirs(deploy_dir, exist_ok=True)

        # Copy the status_server.py template
        template_src = os.path.join(
            os.path.dirname(os.path.abspath(__file__)), "status_server.py"
        )
        dest_script = os.path.join(deploy_dir, "status_server.py")
        shutil.copy2(template_src, dest_script)

        # Write config JSON
        config = {
            "port": int(port),
            "title": title,
            "api_url": api_url,
            "fields": fields,
            "bind_lan_only": bind_lan_only,
        }
        config_path = os.path.join(deploy_dir, "status_config.json")
        with open(config_path, "w") as f:
            json.dump(config, f, indent=2)

        self.log.info(f"Deployed status page at {deploy_dir}")

        # Create and start the service
        self.create_service(
            status_svc,
            exec_start=f"/usr/bin/python3 {dest_script}",
            description=f"{title} Status Page",
            after=f"{svc_name}.service",
        )

    # --- Internal ---

    def _provision_dir(self) -> str:
        """Return the provision directory (where install.py lives inside the container)."""
        # The engine pushes provision/ files to /opt/appstore/provision/
        # __file__ for the install.py lives there at runtime.
        # For subclasses imported from install.py, we look at the runner's module path.
        import sys
        runner_mod = sys.modules.get("app_module")
        if runner_mod and hasattr(runner_mod, "__file__"):
            return os.path.dirname(os.path.abspath(runner_mod.__file__))
        # Fallback to the standard provision target
        return "/opt/appstore/provision"

    @staticmethod
    def _is_progress_line(line: str) -> bool:
        """Detect progress bar lines (pip, curl, wget, etc.) that spam logs."""
        s = line.strip()
        # pip/wget progress bars: |████████████| 90% of 167.3 MiB
        if s.startswith("|") and ("%" in s or "\u2588" in s or "\u25a0" in s):
            return True
        # pip modern progress bars using ━ (U+2501) or ╸ (U+2578)
        if "\u2501" in s or "\u2578" in s:
            return True
        # Lines that are just download sizes: "  7.0/7.0 MB 32.7 MB/s 0:00:00"
        if ("MB/s" in s or "kB/s" in s) and ("/" in s):
            return True
        # Percentage-only lines: 70%, 80%, etc.
        if s.endswith("%") and s[:-1].replace(".", "").isdigit():
            return True
        return False

    def _run(self, cmd: list, check: bool = True, input_text: str = None,
             cwd: str = None, env: dict = None) -> subprocess.CompletedProcess:
        """Run a subprocess, streaming output line-by-line for real-time logging."""
        # Force line-buffered stdout on child processes via _STDBUF_O.
        # Without this, C programs like dpkg/apt use full 4KB buffering when
        # not on a TTY, causing long silent gaps in the provision log.
        run_env = os.environ.copy()
        stdbuf_lib = "/usr/lib/x86_64-linux-gnu/libstdbuf.so"
        if not os.path.exists(stdbuf_lib):
            stdbuf_lib = "/usr/lib/aarch64-linux-gnu/libstdbuf.so"
        if os.path.exists(stdbuf_lib):
            run_env.setdefault("LD_PRELOAD", stdbuf_lib)
            run_env["_STDBUF_O"] = "L"  # line-buffered stdout
        if env:
            run_env.update(env)
        proc = subprocess.Popen(
            cmd,
            stdin=subprocess.PIPE if input_text else None,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,  # line-buffered
            env=run_env,
            cwd=cwd,
        )
        if input_text:
            proc.stdin.write(input_text)
            proc.stdin.close()
        lines = []
        for line in proc.stdout:
            stripped = line.rstrip("\n")
            lines.append(stripped)
            if stripped.strip() and not self._is_progress_line(stripped):
                self.log.info(stripped.strip())
        proc.wait()
        stdout = "\n".join(lines)
        result = subprocess.CompletedProcess(cmd, proc.returncode, stdout=stdout, stderr="")
        if result.returncode != 0:
            cmd_str = " ".join(cmd)
            if check:
                raise RuntimeError(
                    f"Command failed (exit {result.returncode}): {cmd_str}\n{stdout}"
                )
            else:
                # Non-critical: log warning and continue
                self.log.warn(f"Command exited {result.returncode} (non-fatal): {cmd_str}")
        return result
