"""Debian/Ubuntu platform implementations for BaseApp.

Each function takes `app` (a BaseApp instance) as its first argument
and uses app.log, app._run, app.permissions, etc.
"""

import os
import subprocess

from appstore.systemd import generate_service_unit


def pkg_install(app, packages):
    """Install packages via apt-get with error hints."""
    app.log.info(f"Installing packages (apt): {', '.join(packages)}")
    result = app._run(["apt-get", "update", "-qq"], check=False)
    if result.returncode != 0:
        output = result.stdout or ""
        hint = ""
        if "NO_PUBKEY" in output or "not signed" in output:
            hint = (
                "\n\nHint: A GPG key is missing or invalid. Check that add_apt_key() "
                "downloaded the key correctly and the keyring path matches the "
                "signed-by= path in the apt repo line. Use .asc extension for "
                "ASCII-armored keys, .gpg for binary keys."
            )
        elif "Could not resolve" in output:
            hint = "\n\nHint: DNS resolution failed. Check the repository URL."
        raise RuntimeError(
            f"apt-get update failed (exit {result.returncode}): {output}{hint}"
        )
    app._run(["apt-get", "install", "-y", "-qq"] + packages)


def enable_service(app, name):
    """Enable and start a systemd service."""
    app._run(["systemctl", "daemon-reload"])
    app._run(["systemctl", "enable", name])
    app._run(["systemctl", "start", name])


def restart_service(app, name):
    """Restart a systemd service."""
    app._run(["systemctl", "daemon-reload"])
    app._run(["systemctl", "restart", name])


def create_service(app, name, exec_start, description=None,
                   after="network-online.target", user=None,
                   working_directory=None, environment=None,
                   environment_file=None, restart="always",
                   restart_sec=5, type="simple", capabilities=None,
                   extra_unit=None, extra_service=None):
    """Create, enable, and start a systemd service."""
    unit = generate_service_unit(
        name=name, exec_start=exec_start, description=description,
        after=after, user=user, working_directory=working_directory,
        environment=environment, environment_file=environment_file,
        restart=restart, restart_sec=restart_sec, type=type,
        capabilities=capabilities, extra_unit=extra_unit,
        extra_service=extra_service,
    )
    unit_path = f"/etc/systemd/system/{name}.service"
    app.permissions.check_path(unit_path)
    os.makedirs("/etc/systemd/system", exist_ok=True)
    with open(unit_path, "w") as f:
        f.write(unit)
    app.log.info(f"Created systemd service: {name}")
    app._run(["systemctl", "daemon-reload"])
    app._run(["systemctl", "enable", name])
    app._run(["systemctl", "start", name])


def create_user(app, name, system=True, home=None, shell="/bin/false"):
    """Create a user via useradd (Debian)."""
    cmd = ["useradd"]
    if system:
        cmd.append("--system")
    if home:
        cmd.extend(["-m", "-d", home])
    else:
        cmd.append("--no-create-home")
    cmd.extend(["--shell", shell, name])
    result = subprocess.run(cmd, capture_output=True)
    if result.returncode != 0 and b"already exists" not in result.stderr:
        raise RuntimeError(
            f"useradd failed: {result.stderr.decode().strip()}"
        )


def enable_repo(app, repo_name):
    """No-op on Debian — use add_apt_repository() instead."""
    app.log.info(f"enable_repo('{repo_name}') is a no-op on Debian; use add_apt_repository() for APT sources")


def add_apt_key(app, url, keyring_path):
    """Add an APT signing key from a URL."""
    app.log.info(f"Adding APT key from {url}")
    os.makedirs(os.path.dirname(keyring_path) or ".", exist_ok=True)
    dl = subprocess.run(
        ["curl", "-fsSL", url], capture_output=True
    )
    if dl.returncode != 0:
        raise RuntimeError(
            f"Failed to download APT key from {url}: {dl.stderr.decode().strip()}"
        )
    if not dl.stdout:
        raise RuntimeError(f"APT key download returned empty response from {url}")

    if keyring_path.endswith(".gpg"):
        with open(keyring_path, "wb") as f:
            result = subprocess.run(
                ["gpg", "--dearmor"],
                input=dl.stdout,
                stdout=f,
                stderr=subprocess.PIPE,
            )
            if result.returncode != 0:
                raise RuntimeError(
                    f"gpg --dearmor failed: {result.stderr.decode().strip()}"
                )
    else:
        with open(keyring_path, "wb") as f:
            f.write(dl.stdout)


def add_apt_repo(app, repo_line, filename):
    """Add an APT repository source file."""
    dest = f"/etc/apt/sources.list.d/{filename}"
    app.permissions.check_path(dest)
    app.log.info(f"Adding APT repo: {filename}")
    os.makedirs(os.path.dirname(dest), exist_ok=True)
    with open(dest, "w") as f:
        f.write(repo_line + "\n")


def add_apt_repository(app, repo_url, key_url, name="", suite="",
                       components="main"):
    """Add an APT repository with its signing key in one call."""
    import re
    from urllib.parse import urlparse

    if not name:
        parsed = urlparse(repo_url)
        segments = [s for s in parsed.path.strip("/").split("/") if s]
        name = segments[-1] if segments else parsed.hostname.replace(".", "-")
        name = re.sub(r"[^a-zA-Z0-9_-]", "-", name)

    if not suite:
        suite = _detect_codename()

    keyring_path = f"/usr/share/keyrings/{name}.gpg"
    app.add_apt_key(key_url, keyring_path)

    repo_line = f"deb [signed-by={keyring_path}] {repo_url} {suite} {components}"
    app.add_apt_repo(repo_line, f"{name}.list")


def _detect_codename():
    """Detect distro codename from /etc/os-release."""
    try:
        with open("/etc/os-release") as f:
            for line in f:
                if line.startswith("VERSION_CODENAME="):
                    return line.strip().split("=", 1)[1].strip('"')
    except FileNotFoundError:
        pass
    return "stable"
