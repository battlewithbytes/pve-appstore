"""qBittorrent Port Updater — syncs Gluetun forwarded port to qBittorrent."""

from appstore import BaseApp, run


class PortUpdaterApp(BaseApp):
    def install(self):
        qbt_host = self.inputs.string("qbt_host", "127.0.0.1")
        qbt_port = self.inputs.string("qbt_port", "8080")
        qbt_user = self.inputs.string("qbt_user", "admin")
        qbt_password = self.inputs.string("qbt_password", "")
        gluetun_host = self.inputs.string("gluetun_host", "127.0.0.1")
        gluetun_port = self.inputs.string("gluetun_port", "8000")
        poll_interval = self.inputs.string("poll_interval", "60")

        # Install dependencies
        self.apt_install("curl", "jq")

        # Deploy the port updater script
        self.create_dir("/opt/port-updater")
        self.deploy_provision_file(
            "port-updater.sh",
            "/opt/port-updater/port-updater.sh",
            mode="0755",
        )

        # Create and start systemd service with environment variables
        self.create_service(
            "qbt-port-updater",
            exec_start="/opt/port-updater/port-updater.sh",
            description="qBittorrent Port Updater (Gluetun sync)",
            environment={
                "QBT_HOST": qbt_host,
                "QBT_PORT": qbt_port,
                "QBT_USER": qbt_user,
                "QBT_PASSWORD": qbt_password,
                "GLUETUN_HOST": gluetun_host,
                "GLUETUN_PORT": gluetun_port,
                "POLL_INTERVAL": poll_interval,
            },
        )

        self.log.info("qBittorrent Port Updater installed successfully")


run(PortUpdaterApp)
