"""Permission allowlist enforcement for app provisioning.

Every SDK helper method validates its arguments against the app's declared
permissions before executing. Violations raise PermissionDeniedError.
"""

import fnmatch
import json
import os


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

    def check_pip_package(self, package: str) -> None:
        """Verify a pip package is in the allowlist."""
        for allowed in self.pip:
            if fnmatch.fnmatch(package, allowed):
                return
        raise PermissionDeniedError(
            f"pip package '{package}' is not in the allowed pip list: {self.pip}"
        )

    def check_url(self, url: str) -> None:
        """Verify a URL matches an allowed pattern."""
        for pattern in self.urls:
            if fnmatch.fnmatch(url, pattern):
                return
        raise PermissionDeniedError(
            f"URL '{url}' does not match any allowed URL pattern: {self.urls}"
        )

    def check_path(self, path: str) -> None:
        """Verify a filesystem path is under an allowed prefix."""
        normalized = os.path.normpath(path)
        for allowed in self.paths:
            allowed_norm = os.path.normpath(allowed)
            if normalized == allowed_norm or normalized.startswith(allowed_norm + "/"):
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
        """Verify a remote installer script URL is in the allowlist."""
        if url not in self.installer_scripts:
            raise PermissionDeniedError(
                f"installer script '{url}' is not in the allowed installer scripts: {self.installer_scripts}"
            )

    def check_apt_repo(self, repo_line: str) -> None:
        """Verify an APT repository line is in the allowlist.

        First tries exact match (with whitespace normalization), then falls
        back to fnmatch for wildcard patterns. This two-pass approach handles
        literal square brackets in apt repo lines (e.g. [signed-by=...]) which
        fnmatch would otherwise interpret as glob character classes.
        """
        normalized = " ".join(repo_line.split())
        for allowed in self.apt_repos:
            norm_allowed = " ".join(allowed.split())
            # Exact match handles lines with literal brackets
            if normalized == norm_allowed:
                return
            # fnmatch handles wildcard patterns (*, ?)
            if fnmatch.fnmatch(normalized, norm_allowed):
                return
        raise PermissionDeniedError(
            f"APT repo '{repo_line}' is not in the allowed apt repos: {self.apt_repos}"
        )
