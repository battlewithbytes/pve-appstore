"""Tests for permission allowlist enforcement."""

import pytest
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from appstore.permissions import AppPermissions, PermissionDeniedError


def make_perms(**kwargs):
    return AppPermissions(**kwargs)


class TestPackagePermissions:
    def test_allowed_package(self):
        p = make_perms(packages=["nginx", "curl"])
        p.check_package("nginx")  # should not raise

    def test_disallowed_package(self):
        p = make_perms(packages=["nginx"])
        with pytest.raises(PermissionDeniedError, match="apt package 'curl'"):
            p.check_package("curl")

    def test_wildcard_package(self):
        p = make_perms(packages=["jellyfin*"])
        p.check_package("jellyfin")
        p.check_package("jellyfin-server")
        with pytest.raises(PermissionDeniedError):
            p.check_package("nginx")

    def test_empty_package_list(self):
        p = make_perms(packages=[])
        with pytest.raises(PermissionDeniedError):
            p.check_package("anything")


class TestPipPermissions:
    def test_allowed_pip(self):
        p = make_perms(pip=["crawl4ai", "homeassistant"])
        p.check_pip_package("crawl4ai")

    def test_disallowed_pip(self):
        p = make_perms(pip=["crawl4ai"])
        with pytest.raises(PermissionDeniedError, match="pip package 'evil'"):
            p.check_pip_package("evil")


class TestURLPermissions:
    def test_exact_url(self):
        p = make_perms(urls=["https://example.com/file.tar.gz"])
        p.check_url("https://example.com/file.tar.gz")

    def test_wildcard_url(self):
        p = make_perms(urls=["https://downloads.plex.tv/*"])
        p.check_url("https://downloads.plex.tv/plex-keys/PlexSign.key")
        p.check_url("https://downloads.plex.tv/repo/deb/Release")

    def test_disallowed_url(self):
        p = make_perms(urls=["https://example.com/*"])
        with pytest.raises(PermissionDeniedError, match="URL"):
            p.check_url("https://evil.com/malware")


class TestPathPermissions:
    def test_exact_path(self):
        p = make_perms(paths=["/var/www/"])
        p.check_path("/var/www/html/index.html")

    def test_prefix_match(self):
        p = make_perms(paths=["/opt/myapp/"])
        p.check_path("/opt/myapp/config/settings.yaml")

    def test_disallowed_path(self):
        p = make_perms(paths=["/var/www/"])
        with pytest.raises(PermissionDeniedError, match="path"):
            p.check_path("/etc/shadow")

    def test_no_traversal(self):
        p = make_perms(paths=["/opt/myapp/"])
        with pytest.raises(PermissionDeniedError):
            p.check_path("/opt/myapp/../../etc/passwd")

    def test_normalized_path(self):
        p = make_perms(paths=["/opt/myapp"])
        p.check_path("/opt/myapp/file.txt")


class TestServicePermissions:
    def test_allowed_service(self):
        p = make_perms(services=["nginx", "ollama"])
        p.check_service("nginx")

    def test_disallowed_service(self):
        p = make_perms(services=["nginx"])
        with pytest.raises(PermissionDeniedError, match="service 'ssh'"):
            p.check_service("ssh")


class TestUserPermissions:
    def test_allowed_user(self):
        p = make_perms(users=["crawl4ai"])
        p.check_user("crawl4ai")

    def test_disallowed_user(self):
        p = make_perms(users=["crawl4ai"])
        with pytest.raises(PermissionDeniedError, match="user 'root'"):
            p.check_user("root")


class TestCommandPermissions:
    def test_allowed_command(self):
        p = make_perms(commands=["openssl", "ollama"])
        p.check_command("openssl")

    def test_disallowed_command(self):
        p = make_perms(commands=["openssl"])
        with pytest.raises(PermissionDeniedError, match="command 'rm'"):
            p.check_command("rm")

    def test_wildcard_command(self):
        p = make_perms(commands=["/opt/crawl4ai/venv/bin/*"])
        p.check_command("/opt/crawl4ai/venv/bin/crawl4ai-setup")


class TestInstallerScriptPermissions:
    def test_allowed_script(self):
        p = make_perms(installer_scripts=["https://ollama.ai/install.sh"])
        p.check_installer_script("https://ollama.ai/install.sh")

    def test_disallowed_script(self):
        p = make_perms(installer_scripts=["https://ollama.ai/install.sh"])
        with pytest.raises(PermissionDeniedError):
            p.check_installer_script("https://evil.com/backdoor.sh")


class TestAptRepoPermissions:
    def test_allowed_repo(self):
        p = make_perms(apt_repos=["deb https://repo.example.com stable main"])
        p.check_apt_repo("deb https://repo.example.com stable main")

    def test_wildcard_repo(self):
        p = make_perms(apt_repos=["deb *downloads.plex.tv*"])
        p.check_apt_repo("deb https://downloads.plex.tv/repo/deb public main")

    def test_disallowed_repo(self):
        p = make_perms(apt_repos=[])
        with pytest.raises(PermissionDeniedError):
            p.check_apt_repo("deb https://evil.com/repo stable main")


class TestFromFile:
    def test_load_from_json(self, tmp_path):
        import json

        data = {
            "packages": ["nginx"],
            "pip": ["flask"],
            "urls": ["https://example.com/*"],
            "paths": ["/var/www/"],
            "services": ["nginx"],
            "users": [],
            "commands": [],
            "installer_scripts": [],
            "apt_repos": [],
        }
        path = tmp_path / "perms.json"
        path.write_text(json.dumps(data))

        p = AppPermissions.from_file(str(path))
        assert p.packages == ["nginx"]
        assert p.pip == ["flask"]
        p.check_package("nginx")
        p.check_url("https://example.com/file.tar.gz")
