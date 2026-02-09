"""BaseApp abstract class for PVE App Store provisioning.

All app install scripts subclass BaseApp and implement the install() method.
Helper methods enforce the app's declared permission allowlist before executing
any system operations.
"""

import os
import subprocess
import tempfile
from abc import ABC, abstractmethod

from appstore.inputs import AppInputs
from appstore.logging import AppLogger
from appstore.permissions import AppPermissions
from appstore.templates import render


class BaseApp(ABC):
    """Abstract base class for app provisioning scripts."""

    def __init__(self, inputs: AppInputs, permissions: AppPermissions):
        self.inputs = inputs
        self.permissions = permissions
        self.log = AppLogger()

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

    # --- Built-in helpers ---

    def apt_install(self, *packages: str) -> None:
        """Install apt packages. Each package must be in permissions.packages."""
        for pkg in packages:
            self.permissions.check_package(pkg)

        self.log.info(f"Installing apt packages: {', '.join(packages)}")
        self._run(["apt-get", "update", "-qq"])
        self._run(["apt-get", "install", "-y", "-qq"] + list(packages))

    def write_config(self, path: str, template_str: str, **kwargs) -> str:
        """Write a config file using string.Template substitution.

        Args:
            path: Destination path (must be under an allowed path prefix).
            template_str: Template string using $variable syntax.
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

    def enable_service(self, name: str) -> None:
        """Enable and start a systemd service."""
        self.permissions.check_service(name)
        self.log.info(f"Enabling service: {name}")
        self._run(["systemctl", "daemon-reload"])
        self._run(["systemctl", "enable", name])
        self._run(["systemctl", "start", name])

    def restart_service(self, name: str) -> None:
        """Restart a systemd service."""
        self.permissions.check_service(name)
        self.log.info(f"Restarting service: {name}")
        self._run(["systemctl", "daemon-reload"])
        self._run(["systemctl", "restart", name])

    def create_dir(self, path: str, owner: str = None, mode: str = "0755") -> None:
        """Create a directory with optional ownership."""
        self.permissions.check_path(path)
        os.makedirs(path, exist_ok=True)
        os.chmod(path, int(mode, 8))
        if owner:
            self._run(["chown", owner, path])
        self.log.info(f"Created directory: {path}")

    def download(self, url: str, dest: str) -> None:
        """Download a URL to a file."""
        self.permissions.check_url(url)
        self.permissions.check_path(dest)
        self.log.info(f"Downloading {url} -> {dest}")
        os.makedirs(os.path.dirname(dest) or ".", exist_ok=True)
        self._run(["curl", "-fsSL", "-o", dest, url])

    def create_user(
        self,
        name: str,
        system: bool = True,
        home: str = None,
        shell: str = "/bin/false",
    ) -> None:
        """Create a system user."""
        self.permissions.check_user(name)
        self.log.info(f"Creating user: {name}")
        cmd = ["useradd"]
        if system:
            cmd.append("--system")
        if home:
            cmd.extend(["-m", "-d", home])
        else:
            cmd.append("--no-create-home")
        cmd.extend(["--shell", shell, name])
        # Ignore errors if user already exists
        result = subprocess.run(cmd, capture_output=True)
        if result.returncode != 0 and b"already exists" not in result.stderr:
            raise RuntimeError(
                f"useradd failed: {result.stderr.decode().strip()}"
            )

    def pip_install(self, *packages: str, venv: str = None) -> None:
        """Install pip packages, optionally in a venv."""
        for pkg in packages:
            self.permissions.check_pip_package(pkg)

        pip_bin = f"{venv}/bin/pip" if venv else "pip3"
        self.log.info(f"Installing pip packages: {', '.join(packages)}")
        self._run([pip_bin, "install", "--progress-bar", "off"] + list(packages))

    def create_venv(self, path: str) -> None:
        """Create a Python virtual environment."""
        self.permissions.check_path(path)
        self.log.info(f"Creating venv: {path}")
        self._run(["python3", "-m", "venv", path])
        # Upgrade pip in the new venv
        self._run([f"{path}/bin/pip", "install", "--progress-bar", "off", "-U", "pip"])

    def add_apt_key(self, url: str, keyring_path: str) -> None:
        """Add an APT signing key from a URL."""
        self.permissions.check_url(url)
        self.permissions.check_path(keyring_path)
        self.log.info(f"Adding APT key from {url}")
        os.makedirs(os.path.dirname(keyring_path) or ".", exist_ok=True)
        # Download key and dearmor it
        dl = subprocess.run(
            ["curl", "-fsSL", url], capture_output=True, check=True
        )
        with open(keyring_path, "wb") as f:
            result = subprocess.run(
                ["gpg", "--dearmor"],
                input=dl.stdout,
                stdout=f,
                stderr=subprocess.PIPE,
            )

    def add_apt_repo(self, repo_line: str, filename: str) -> None:
        """Add an APT repository source file."""
        self.permissions.check_apt_repo(repo_line)
        dest = f"/etc/apt/sources.list.d/{filename}"
        self.permissions.check_path(dest)
        self.log.info(f"Adding APT repo: {filename}")
        with open(dest, "w") as f:
            f.write(repo_line + "\n")

    def run_command(self, cmd: list, check: bool = True) -> subprocess.CompletedProcess:
        """Run a command. The binary (cmd[0]) must be in permissions.commands."""
        if not cmd:
            raise ValueError("empty command")
        self.permissions.check_command(cmd[0])
        return self._run(cmd, check=check)

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

    def chown(self, path: str, owner: str, recursive: bool = False) -> None:
        """Change file/directory ownership."""
        self.permissions.check_path(path)
        cmd = ["chown"]
        if recursive:
            cmd.append("-R")
        cmd.extend([owner, path])
        self._run(cmd)

    # --- Internal ---

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

    def _run(self, cmd: list, check: bool = True) -> subprocess.CompletedProcess:
        """Run a subprocess, streaming output line-by-line for real-time logging."""
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,  # line-buffered
        )
        lines = []
        for line in proc.stdout:
            stripped = line.rstrip("\n")
            lines.append(stripped)
            if stripped.strip() and not self._is_progress_line(stripped):
                self.log.info(stripped.strip())
        proc.wait()
        stdout = "\n".join(lines)
        result = subprocess.CompletedProcess(cmd, proc.returncode, stdout=stdout, stderr="")
        if check and result.returncode != 0:
            raise RuntimeError(
                f"Command failed (exit {result.returncode}): {' '.join(cmd)}\n{stdout}"
            )
        return result
