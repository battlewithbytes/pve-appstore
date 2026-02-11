"""qBittorrent — lightweight BitTorrent client with WebUI (Alpine Linux)."""

import base64
import hashlib
import os

from appstore import BaseApp, run


class QBittorrentApp(BaseApp):
    def install(self):
        webui_port = self.inputs.string("webui_port", "8080")
        torrent_port = self.inputs.string("torrent_port", "6881")
        download_path = self.inputs.string("download_path", "/downloads")
        password = self.inputs.string("initial_password", "changeme")

        # Enable Alpine community repository (required for qbittorrent-nox)
        self.run_command([
            "sed", "-i", "s/#.*community/community/", "/etc/apk/repositories",
        ])

        # Install packages via OS-aware helper (apk on Alpine)
        self.pkg_install("qbittorrent-nox", "python3", "p7zip")

        # Create service user
        self.create_user("qbittorrent", system=True, home="/var/lib/qbittorrent")

        # Create directories
        config_dir = "/var/lib/qbittorrent/.config/qBittorrent"
        self.create_dir(config_dir)
        self.create_dir(download_path)
        self.create_dir(f"{download_path}/incomplete")

        # Generate PBKDF2 password hash for qBittorrent config
        salt = os.urandom(16)
        dk = hashlib.pbkdf2_hmac("sha512", password.encode(), salt, 100000)
        password_hash = (
            f"@ByteArray({base64.b64encode(salt).decode()}"
            f":{base64.b64encode(dk).decode()})"
        )

        # Read config template and write with substituted values
        template = self.provision_file("qBittorrent.conf")
        self.write_config(
            f"{config_dir}/qBittorrent.conf",
            template,
            torrent_port=torrent_port,
            download_path=download_path,
            webui_port=webui_port,
            password_hash=password_hash,
        )

        # Set ownership of all qbittorrent data
        self.chown("/var/lib/qbittorrent", "qbittorrent:qbittorrent", recursive=True)

        # Create and start OpenRC service
        self.create_service(
            "qbittorrent-nox",
            exec_start=(
                f"/usr/bin/qbittorrent-nox"
                f" --webui-port={webui_port}"
                f" --torrenting-port={torrent_port}"
            ),
            description="qBittorrent-nox BitTorrent client",
            user="qbittorrent",
            environment={
                "HOME": "/var/lib/qbittorrent",
                "XDG_CONFIG_HOME": "/var/lib/qbittorrent/.config",
                "XDG_DATA_HOME": "/var/lib/qbittorrent/.local/share",
            },
        )

        # Emit outputs
        self.log.output("webui_password", password)
        self.log.info("qBittorrent installed successfully")


run(QBittorrentApp)
