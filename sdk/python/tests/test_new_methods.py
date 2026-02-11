"""Tests for SDK v2 high-level abstractions."""

import json
import os
import sys
from unittest.mock import patch, MagicMock, mock_open

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from appstore.base import BaseApp
from appstore.inputs import AppInputs
from appstore.permissions import AppPermissions, PermissionDeniedError
from appstore.osdetect import detect_os
from appstore.systemd import generate_service_unit
from appstore.openrc import generate_init_script
from appstore.oci import OCIClient


def mock_popen_factory(returncode=0):
    """Create a mock subprocess.Popen that simulates line-by-line streaming."""
    def make_popen(*args, **kwargs):
        mock_proc = MagicMock()
        mock_proc.stdout = iter([])
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
             inputs=None, os_type="debian"):
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
    with patch("appstore.base.detect_os", return_value=os_type):
        return DummyApp(AppInputs(inputs or {}), perms)


# --- OS Detection ---

class TestDetectOS:
    def test_debian(self, tmp_path):
        os_release = tmp_path / "os-release"
        os_release.write_text('ID=debian\nVERSION_ID="12"\n')
        with patch("builtins.open", mock_open(read_data='ID=debian\nVERSION_ID="12"\n')):
            result = detect_os()
        assert result == "debian"

    def test_ubuntu(self, tmp_path):
        content = 'ID=ubuntu\nID_LIKE=debian\n'
        with patch("builtins.open", mock_open(read_data=content)):
            result = detect_os()
        assert result == "debian"

    def test_alpine(self):
        content = 'ID=alpine\nVERSION_ID=3.20\n'
        with patch("builtins.open", mock_open(read_data=content)):
            result = detect_os()
        assert result == "alpine"

    def test_unknown(self):
        content = 'ID=fedora\n'
        with patch("builtins.open", mock_open(read_data=content)):
            result = detect_os()
        assert result == "unknown"

    def test_missing_file(self):
        with patch("builtins.open", side_effect=FileNotFoundError):
            result = detect_os()
        assert result == "unknown"


# --- systemd.py ---

class TestGenerateServiceUnit:
    def test_basic_unit(self):
        unit = generate_service_unit(
            name="myapp",
            exec_start="/usr/bin/myapp",
        )
        assert "[Unit]" in unit
        assert "Description=myapp" in unit
        assert "[Service]" in unit
        assert "ExecStart=/usr/bin/myapp" in unit
        assert "Type=simple" in unit
        assert "Restart=always" in unit
        assert "RestartSec=5" in unit
        assert "[Install]" in unit
        assert "WantedBy=multi-user.target" in unit

    def test_full_unit(self):
        unit = generate_service_unit(
            name="gluetun",
            exec_start="/etc/gluetun/start.sh",
            description="Gluetun VPN Client",
            after="network-online.target",
            user="vpn",
            working_directory="/etc/gluetun",
            environment={"FOO": "bar", "BAZ": "qux"},
            environment_file="/etc/gluetun/env",
            restart="on-failure",
            restart_sec=10,
            type="simple",
            capabilities=["CAP_NET_ADMIN", "CAP_NET_RAW"],
            extra_unit="Documentation=https://example.com",
            extra_service="LimitNOFILE=65535",
        )
        assert "Description=Gluetun VPN Client" in unit
        assert "After=network-online.target" in unit
        assert "User=vpn" in unit
        assert "WorkingDirectory=/etc/gluetun" in unit
        assert 'Environment="FOO=bar"' in unit
        assert 'Environment="BAZ=qux"' in unit
        assert "EnvironmentFile=/etc/gluetun/env" in unit
        assert "Restart=on-failure" in unit
        assert "RestartSec=10" in unit
        assert "AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW" in unit
        assert "CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW" in unit
        assert "Documentation=https://example.com" in unit
        assert "LimitNOFILE=65535" in unit

    def test_no_after(self):
        unit = generate_service_unit(
            name="myapp", exec_start="/usr/bin/myapp", after=None,
        )
        assert "After=" not in unit
        assert "Wants=" not in unit


# --- openrc.py ---

class TestGenerateInitScript:
    def test_basic_script(self):
        script = generate_init_script(
            name="myapp",
            exec_start="/usr/bin/myapp",
        )
        assert "#!/sbin/openrc-run" in script
        assert "command=/usr/bin/myapp" in script
        assert "command_background=true" in script
        assert "pidfile=/run/myapp.pid" in script
        assert "need net" in script

    def test_with_user_and_env(self):
        script = generate_init_script(
            name="myapp",
            exec_start="/usr/bin/myapp",
            user="appuser",
            environment={"KEY": "val"},
            environment_file="/etc/myapp/env",
        )
        assert "command_user=appuser" in script
        assert 'export KEY="val"' in script
        assert '. "/etc/myapp/env"' in script

    def test_restart_always_enables_supervisor(self):
        script = generate_init_script(
            name="myapp", exec_start="/usr/bin/myapp", restart="always",
        )
        assert "supervisor=supervise-daemon" in script


# --- OCI Client ---

class TestOCIClient:
    def test_detect_arch_amd64(self):
        client = OCIClient()
        with patch("appstore.oci.platform.machine", return_value="x86_64"):
            assert client._detect_arch() == "amd64"

    def test_detect_arch_arm64(self):
        client = OCIClient()
        with patch("appstore.oci.platform.machine", return_value="aarch64"):
            assert client._detect_arch() == "arm64"

    def test_detect_arch_unsupported(self):
        client = OCIClient()
        with patch("appstore.oci.platform.machine", return_value="mips"):
            with pytest.raises(RuntimeError, match="Unsupported architecture"):
                client._detect_arch()


# --- pkg_install ---

class TestPkgInstall:
    @patch("appstore.base.subprocess.Popen")
    def test_debian_uses_apt(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(packages=["nginx", "curl"], os_type="debian")
        app.pkg_install("nginx", "curl")

        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["apt-get", "update", "-qq"] in cmds
        assert ["apt-get", "install", "-y", "-qq", "nginx", "curl"] in cmds

    @patch("appstore.base.subprocess.Popen")
    def test_alpine_uses_apk(self, mock_popen):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(packages=["nginx", "curl"], os_type="alpine")
        app.pkg_install("nginx", "curl")

        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["apk", "add", "--no-cache", "nginx", "curl"] in cmds

    def test_rejects_disallowed_package(self):
        app = make_app(packages=["nginx"], os_type="debian")
        with pytest.raises(PermissionDeniedError, match="apt package 'evil'"):
            app.pkg_install("evil")


# --- create_service ---

class TestCreateService:
    @patch("appstore.base.subprocess.Popen")
    def test_debian_creates_systemd_unit(self, mock_popen, tmp_path):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(
            services=["myapp"],
            paths=["/etc/systemd/system"],
            os_type="debian",
        )
        with patch("appstore.base.os.makedirs"):
            with patch("builtins.open", mock_open()):
                app.create_service("myapp", exec_start="/usr/bin/myapp")

        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["systemctl", "daemon-reload"] in cmds
        assert ["systemctl", "enable", "myapp"] in cmds
        assert ["systemctl", "start", "myapp"] in cmds

    @patch("appstore.base.subprocess.Popen")
    def test_alpine_creates_openrc_script(self, mock_popen, tmp_path):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(
            services=["myapp"],
            paths=["/etc/init.d"],
            os_type="alpine",
        )
        with patch("appstore.base.os.makedirs"):
            with patch("builtins.open", mock_open()):
                with patch("appstore.base.os.chmod"):
                    app.create_service("myapp", exec_start="/usr/bin/myapp")

        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["rc-update", "add", "myapp", "default"] in cmds
        assert ["rc-service", "myapp", "start"] in cmds

    def test_rejects_disallowed_service(self):
        app = make_app(services=["nginx"], os_type="debian")
        with pytest.raises(PermissionDeniedError, match="service 'evil'"):
            app.create_service("evil", exec_start="/usr/bin/evil")


# --- wait_for_http ---

class TestWaitForHttp:
    @patch("appstore.base.urllib.request.urlopen")
    @patch("appstore.base.time.sleep")
    def test_returns_true_on_200(self, mock_sleep, mock_urlopen):
        mock_resp = MagicMock()
        mock_resp.status = 200
        mock_urlopen.return_value = mock_resp
        app = make_app(urls=["http://127.0.0.1:8000/*"])
        result = app.wait_for_http("http://127.0.0.1:8000/health", timeout=10)
        assert result is True
        mock_sleep.assert_not_called()

    @patch("appstore.base.urllib.request.urlopen")
    @patch("appstore.base.time.sleep")
    def test_returns_false_on_timeout(self, mock_sleep, mock_urlopen):
        mock_urlopen.side_effect = Exception("Connection refused")
        app = make_app(urls=["http://127.0.0.1:8000/*"])
        result = app.wait_for_http("http://127.0.0.1:8000/health", timeout=6, interval=3)
        assert result is False

    def test_rejects_disallowed_url(self):
        app = make_app(urls=["http://127.0.0.1:8000/*"])
        with pytest.raises(PermissionDeniedError, match="URL"):
            app.wait_for_http("http://evil.com/")


# --- write_env_file ---

class TestWriteEnvFile:
    def test_writes_env_vars(self, tmp_path):
        path = str(tmp_path / "env")
        app = make_app(paths=[str(tmp_path)])
        app.write_env_file(path, {
            "KEY1": "value1",
            "KEY2": "value2",
            "EMPTY": "",
            "NONE_VAL": None,
        })
        with open(path) as f:
            content = f.read()
        assert "KEY1=value1\n" in content
        assert "KEY2=value2\n" in content
        assert "EMPTY" not in content
        assert "NONE_VAL" not in content

    def test_sets_permissions(self, tmp_path):
        path = str(tmp_path / "env")
        app = make_app(paths=[str(tmp_path)])
        app.write_env_file(path, {"A": "b"}, mode="0600")
        stat = os.stat(path)
        assert oct(stat.st_mode & 0o777) == "0o600"

    def test_rejects_disallowed_path(self):
        app = make_app(paths=["/var/www/"])
        with pytest.raises(PermissionDeniedError):
            app.write_env_file("/etc/secret", {"KEY": "val"})


# --- sysctl ---

class TestSysctl:
    @patch("appstore.base.subprocess.Popen")
    def test_writes_sysctl_conf(self, mock_popen, tmp_path):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(paths=["/etc/sysctl.d"])
        with patch("appstore.base.os.makedirs"):
            with patch("builtins.open", mock_open()) as mf:
                app.sysctl({"net.ipv6.conf.all.disable_ipv6": 1})

        # Verify sysctl --system was called
        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["sysctl", "--system"] in cmds

    @patch("appstore.base.subprocess.Popen")
    def test_disable_ipv6(self, mock_popen, tmp_path):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(paths=["/etc/sysctl.d"])
        with patch("appstore.base.os.makedirs"):
            with patch("builtins.open", mock_open()) as mf:
                app.disable_ipv6()

        # Verify sysctl --system was called
        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["sysctl", "--system"] in cmds


# --- status_page ---

class TestStatusPage:
    @patch("appstore.base.subprocess.Popen")
    @patch("appstore.base.shutil.copy2")
    def test_deploys_status_page(self, mock_copy, mock_popen, tmp_path):
        mock_popen.side_effect = mock_popen_factory()
        app = make_app(
            urls=["http://127.0.0.1:8000/*"],
            services=["test-app-status"],
            paths=["/etc/test-app-status", "/etc/systemd/system"],
            os_type="debian",
        )
        with patch("appstore.base.os.makedirs"):
            with patch("builtins.open", mock_open()):
                app.status_page(
                    port=8001,
                    title="Test App",
                    api_url="http://127.0.0.1:8000/api",
                    fields={"ip": "IP Address"},
                )

        # Should have copied the template
        assert mock_copy.called

        # Should have created a service
        cmds = [c[0][0] for c in mock_popen.call_args_list]
        assert ["systemctl", "daemon-reload"] in cmds


# --- pull_oci_binary ---

class TestPullOciBinary:
    @patch("appstore.base.subprocess.run")
    @patch("appstore.oci.OCIClient.pull_binary")
    def test_calls_oci_client(self, mock_pull, mock_run):
        mock_run.return_value = MagicMock(returncode=0, stdout="statically linked")
        app = make_app(
            urls=["https://auth.docker.io/*", "https://registry-1.docker.io/*"],
            paths=["/usr/local/bin"],
        )
        app.pull_oci_binary("myimage", "/usr/local/bin/mybin")
        mock_pull.assert_called_once_with("myimage", "/usr/local/bin/mybin", tag="latest")

    def test_rejects_disallowed_url(self):
        app = make_app(urls=[], paths=["/tmp/"])
        with pytest.raises(PermissionDeniedError, match="URL"):
            app.pull_oci_binary("myimage", "/tmp/mybin")

    @patch("appstore.base.subprocess.run")
    @patch("appstore.oci.OCIClient.pull_binary")
    def test_warns_on_missing_libs(self, mock_pull, mock_run):
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="libfoo.so => not found\nlibbar.so => /lib/libbar.so\n",
        )
        app = make_app(
            urls=["https://auth.docker.io/*", "https://registry-1.docker.io/*"],
            paths=["/usr/local/bin"],
        )
        # Should not raise, just warn
        app.pull_oci_binary("myimage", "/usr/local/bin/mybin")
        mock_pull.assert_called_once()


# --- provision_file / deploy_provision_file ---

class TestProvisionFile:
    def test_reads_provision_file(self, tmp_path):
        # Create a fake provision file
        prov_file = tmp_path / "template.conf"
        prov_file.write_text("server_name example.com;")
        app = make_app()
        with patch.object(app, "_provision_dir", return_value=str(tmp_path)):
            content = app.provision_file("template.conf")
        assert content == "server_name example.com;"

    def test_deploy_provision_file(self, tmp_path):
        # Create a fake provision file
        prov_file = tmp_path / "script.py"
        prov_file.write_text("print('hello')")
        dest = str(tmp_path / "deployed" / "script.py")
        app = make_app(paths=[str(tmp_path)])
        with patch.object(app, "_provision_dir", return_value=str(tmp_path)):
            app.deploy_provision_file("script.py", dest, mode="0755")
        assert os.path.exists(dest)
        with open(dest) as f:
            assert f.read() == "print('hello')"
        assert oct(os.stat(dest).st_mode & 0o777) == "0o755"

    def test_deploy_rejects_disallowed_path(self):
        app = make_app(paths=["/var/www/"])
        with pytest.raises(PermissionDeniedError):
            app.deploy_provision_file("evil.py", "/etc/shadow")


# --- create_user Alpine ---

class TestCreateUserAlpine:
    def test_alpine_uses_adduser(self):
        app = make_app(users=["testuser"], os_type="alpine")
        with patch("appstore.base.subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stderr=b"")
            app.create_user("testuser", system=True, home="/opt/testuser")
        cmd = mock_run.call_args[0][0]
        assert cmd[0] == "adduser"
        assert "-D" in cmd
        assert "-S" in cmd
        assert "-h" in cmd
        assert "/opt/testuser" in cmd

    def test_debian_uses_useradd(self):
        app = make_app(users=["testuser"], os_type="debian")
        with patch("appstore.base.subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stderr=b"")
            app.create_user("testuser", system=True)
        cmd = mock_run.call_args[0][0]
        assert cmd[0] == "useradd"
        assert "--system" in cmd


# --- status_server.py template ---

class TestStatusServerTemplate:
    def test_template_is_importable(self):
        """Verify the status_server.py template has valid Python syntax."""
        import py_compile
        template_path = os.path.join(
            os.path.dirname(__file__), "..", "appstore", "status_server.py"
        )
        # This will raise SyntaxError if the template has bad syntax
        py_compile.compile(template_path, doraise=True)
