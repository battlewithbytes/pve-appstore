"""Tests for BaseApp helper methods using mocked subprocess."""

import os
import sys
from unittest.mock import patch, MagicMock, call

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from appstore.base import BaseApp
from appstore.inputs import AppInputs
from appstore.permissions import AppPermissions, PermissionDeniedError


def mock_popen_factory(returncode=0):
    """Create a mock subprocess.Popen that simulates line-by-line streaming."""
    def make_popen(*args, **kwargs):
        mock_proc = MagicMock()
        mock_proc.stdout = iter([])  # no output lines
        mock_proc.returncode = returncode
        mock_proc.wait.return_value = None
        return mock_proc
    return make_popen


class DummyApp(BaseApp):
    """Concrete subclass for testing."""

    def install(self):
        pass


def make_app(packages=None, pip=None, urls=None, paths=None, services=None,
             users=None, commands=None, installer_scripts=None, apt_repos=None,
             inputs=None):
    perms = AppPermissions(
        packages=packages or [],
        pip=pip or [],
        urls=urls or [],
        paths=paths or [],
        services=services or [],
        users=users or [],
        commands=commands or [],
        installer_scripts=installer_scripts or [],
        apt_repos=apt_repos or [],
    )
    return DummyApp(AppInputs(inputs or {}), perms)


class TestAptInstall:
    @patch("appstore.base.subprocess.Popen")
    def test_installs_allowed_packages(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(packages=["nginx", "curl"])
        app.apt_install("nginx", "curl")

        # Should call apt-get update then apt-get install
        assert mock_popen.call_count == 2
        install_call = mock_popen.call_args_list[1]
        assert install_call[0][0] == ["apt-get", "install", "-y", "nginx", "curl"]

    def test_rejects_disallowed_packages(self):
        app = make_app(packages=["nginx"])
        with pytest.raises(PermissionDeniedError, match="apt package 'evil'"):
            app.apt_install("evil")


class TestWriteConfig:
    def test_writes_template(self, tmp_path):
        path = str(tmp_path / "config.conf")
        app = make_app(paths=[str(tmp_path)])
        app.write_config(path, "server_name $domain;", domain="example.com")
        with open(path) as f:
            assert f.read() == "server_name example.com;"

    def test_rejects_disallowed_path(self):
        app = make_app(paths=["/var/www/"])
        with pytest.raises(PermissionDeniedError, match="path"):
            app.write_config("/etc/shadow", "evil")

    def test_safe_substitute_missing_var(self, tmp_path):
        path = str(tmp_path / "config.conf")
        app = make_app(paths=[str(tmp_path)])
        app.write_config(path, "port=$port host=$host", port="8080")
        with open(path) as f:
            content = f.read()
        assert "8080" in content
        assert "$host" in content  # safe_substitute leaves missing vars


class TestEnableService:
    @patch("appstore.base.subprocess.Popen")
    def test_enables_allowed_service(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(services=["nginx"])
        app.enable_service("nginx")

        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["systemctl", "daemon-reload"] in cmds
        assert ["systemctl", "enable", "nginx"] in cmds
        assert ["systemctl", "start", "nginx"] in cmds

    def test_rejects_disallowed_service(self):
        app = make_app(services=["nginx"])
        with pytest.raises(PermissionDeniedError, match="service 'ssh'"):
            app.enable_service("ssh")


class TestCreateDir:
    def test_creates_directory(self, tmp_path):
        target = str(tmp_path / "subdir" / "nested")
        app = make_app(paths=[str(tmp_path)])
        with patch("appstore.base.subprocess.Popen") as mock_popen:
            mock_popen.side_effect = mock_popen_factory()
            app.create_dir(target, mode="0755")
        assert os.path.isdir(target)

    def test_rejects_disallowed_dir(self):
        app = make_app(paths=["/var/www/"])
        with pytest.raises(PermissionDeniedError):
            app.create_dir("/etc/evil")


class TestDownload:
    @patch("appstore.base.subprocess.Popen")
    def test_downloads_allowed_url(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(
            urls=["https://example.com/*"],
            paths=["/tmp/"],
        )
        app.download("https://example.com/file.tar.gz", "/tmp/file.tar.gz")
        curl_call = mock_popen.call_args_list[-1]
        assert "curl" in curl_call[0][0]

    def test_rejects_disallowed_url(self):
        app = make_app(urls=["https://example.com/*"], paths=["/tmp/"])
        with pytest.raises(PermissionDeniedError, match="URL"):
            app.download("https://evil.com/malware", "/tmp/malware")


class TestRunCommand:
    @patch("appstore.base.subprocess.Popen")
    def test_runs_allowed_command(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(commands=["openssl"])
        app.run_command(["openssl", "req", "-x509"])
        assert mock_popen.called
        assert mock_popen.call_args[0][0][0] == "openssl"

    def test_rejects_disallowed_command(self):
        app = make_app(commands=["openssl"])
        with pytest.raises(PermissionDeniedError, match="command 'rm'"):
            app.run_command(["rm", "-rf", "/"])

    def test_empty_command(self):
        app = make_app()
        with pytest.raises(ValueError, match="empty command"):
            app.run_command([])


class TestRunInstallerScript:
    @patch("appstore.base.subprocess.Popen")
    @patch("appstore.base.os.chmod")
    @patch("appstore.base.os.unlink")
    def test_runs_allowed_script(self, mock_unlink, mock_chmod, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(installer_scripts=["https://ollama.ai/install.sh"])
        app.run_installer_script("https://ollama.ai/install.sh")
        assert mock_popen.call_count == 2  # curl + bash

    def test_rejects_disallowed_script(self):
        app = make_app(installer_scripts=["https://ollama.ai/install.sh"])
        with pytest.raises(PermissionDeniedError):
            app.run_installer_script("https://evil.com/backdoor.sh")


class TestPipInstall:
    @patch("appstore.base.subprocess.Popen")
    def test_installs_allowed_pip(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(pip=["crawl4ai"])
        app.pip_install("crawl4ai", venv="/opt/app/venv")
        install_call = mock_popen.call_args_list[-1]
        assert install_call[0][0] == ["/opt/app/venv/bin/pip", "install", "crawl4ai"]

    def test_rejects_disallowed_pip(self):
        app = make_app(pip=["crawl4ai"])
        with pytest.raises(PermissionDeniedError, match="pip package 'evil'"):
            app.pip_install("evil")
