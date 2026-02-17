"""Permission allowlist enforcement for app provisioning.

Every SDK helper method validates its arguments against the app's declared
permissions before executing. Violations raise PermissionDeniedError.
"""

import fnmatch
import json
import os
import re


class PermissionDeniedError(Exception):
    """Raised when an app attempts an action not in its permission allowlist."""
    pass


class AppPermissions:
    """Enforces the permission allowlist declared in the app manifest."""

    def __init__(
        self,
        packages: list = None,
        pip: list = None,
        urls: list = None,
        paths: list = None,
        services: list = None,
        users: list = None,
        commands: list = None,
        installer_scripts: list = None,
        apt_repos: list = None,
    ):
        self.packages = packages or []
        self.pip = pip or []
        self.urls = urls or []
        self.paths = paths or []
        self.services = services or []
        self.users = users or []
        self.commands = commands or []
        self.installer_scripts = installer_scripts or []
        self.apt_repos = apt_repos or []

    @classmethod
    def from_file(cls, path: str) -> "AppPermissions":
        with open(path, "r") as f:
            data = json.load(f)
        return cls(**data)

    def check_package(self, package: str) -> None:
        """Verify an apt package is in the allowlist."""
        for allowed in self.packages:
            if fnmatch.fnmatch(package, allowed):
                return
        raise PermissionDeniedError(
            f"apt package '{package}' is not in the allowed packages list: {self.packages}"
        )

    @staticmethod
    def _pip_base_name(pkg: str) -> str:
        """Strip extras and version specifiers from a pip package string.

        e.g. "josepy<2" -> "josepy", "homeassistant[all]>=2024.1" -> "homeassistant"
        """
        return re.split(r"[\[<>=!~;@]", pkg)[0].strip()

    def check_pip_package(self, package: str) -> None:
        """Verify a pip package is in the allowlist.

        Version specifiers and extras are stripped before matching,
        so "josepy<2" matches an allowlist entry of "josepy".
        """
        base = self._pip_base_name(package)
        for allowed in self.pip:
            allowed_base = self._pip_base_name(allowed)
            if fnmatch.fnmatch(base, allowed_base):
                return
        raise PermissionDeniedError(
            f"pip package '{base}' (from '{package}') is not in the allowed pip list: {self.pip}"
        )

    def check_url(self, url: str) -> None:
        """Verify a URL matches an allowed pattern."""
        for pattern in self.urls:
            if fnmatch.fnmatch(url, pattern):
                return
        raise PermissionDeniedError(
            f"URL '{url}' does not match any allowed URL pattern: {self.urls}"
        )

    # Paths implicitly allowed as scratch space (no manifest entry needed)
    _implicit_paths = ["/tmp", "/opt/venv"]

    def check_path(self, path: str) -> None:
        """Verify a filesystem path is under an allowed prefix."""
        normalized = os.path.normpath(path)
        for allowed in self._implicit_paths + self.paths:
            allowed_norm = os.path.normpath(allowed)
            # Root "/" allows all absolute paths
            if allowed_norm == "/":
                if normalized.startswith("/"):
                    return
            elif normalized == allowed_norm or normalized.startswith(allowed_norm + "/"):
                return
        raise PermissionDeniedError(
            f"path '{path}' is not under any allowed path: {self.paths}"
        )

    def check_service(self, service: str) -> None:
        """Verify a systemd service is in the allowlist."""
        if service not in self.services:
            raise PermissionDeniedError(
                f"service '{service}' is not in the allowed services list: {self.services}"
            )

    def check_user(self, user: str) -> None:
        """Verify a system user is in the allowlist."""
        if user not in self.users:
            raise PermissionDeniedError(
                f"user '{user}' is not in the allowed users list: {self.users}"
            )

    def check_command(self, cmd: str) -> None:
        """Verify a command binary is in the allowlist."""
        for allowed in self.commands:
            if fnmatch.fnmatch(cmd, allowed):
                return
        raise PermissionDeniedError(
            f"command '{cmd}' is not in the allowed commands list: {self.commands}"
        )

    def check_installer_script(self, url: str) -> None:
        """Verify a remote installer script URL is in the allowlist.

        Checks against permissions.urls (with glob matching).
        The separate installer_scripts field is deprecated — use urls instead.
        """
        # Check urls first (glob-aware)
        for pattern in self.urls:
            if fnmatch.fnmatch(url, pattern):
                return
        # Fall back to legacy installer_scripts exact match
        if url in self.installer_scripts:
            return
        raise PermissionDeniedError(
            f"installer script '{url}' is not in the allowed URL patterns: {self.urls}"
        )

    @staticmethod
    def _extract_repo_url(line: str) -> str:
        """Extract the URL from a deb repo line (e.g. 'deb [opts] URL suite component')."""
        for token in line.split():
            if token.startswith("http://") or token.startswith("https://"):
                return token.rstrip("/")
        return ""

    def check_apt_repo(self, repo_line: str) -> None:
        """Verify an APT repository line is in the allowlist.

        Matches are checked in order:
        1. URL extraction — the repo URL from the deb line is matched against
           allowed entries (which can be bare URLs or fnmatch patterns).
        2. Full-line exact match (legacy, with whitespace normalization).
        """
        repo_url = self._extract_repo_url(repo_line)
        for allowed in self.apt_repos:
            allowed_clean = allowed.rstrip("/")
            # URL-based match: allowed is a URL or URL pattern
            if repo_url and (
                repo_url == allowed_clean
                or repo_url.startswith(allowed_clean + "/")
                or fnmatch.fnmatch(repo_url, allowed_clean)
            ):
                return
            # Legacy: full-line exact match for backwards compatibility
            normalized = " ".join(repo_line.split())
            norm_allowed = " ".join(allowed.split())
            if normalized == norm_allowed:
                return
        raise PermissionDeniedError(
            f"APT repo URL '{repo_url or repo_line}' is not in the allowed apt repos: {self.apt_repos}"
        )
