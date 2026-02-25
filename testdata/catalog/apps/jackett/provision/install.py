"""Jackett — API proxy server for torrent trackers."""

from appstore import BaseApp, run


class JackettApp(BaseApp):
    def install(self):
        webui_port = self.inputs.string("webui_port", "9117")

        # Install .NET runtime dependencies
        self.apt_install("curl", "libicu72", "libssl3")

        # Create service user
        self.create_user("jackett", system=True, home="/var/lib/jackett")

        # Download latest Jackett release from GitHub
        self.github_download_release(
            "Jackett", "Jackett", "LinuxAMDx64.tar.gz", "/tmp/jackett.tar.gz",
        )

        # Extract to /opt
        self.extract_tar("/tmp/jackett.tar.gz", "/opt")

        # Set ownership
        self.chown("/opt/Jackett", "jackett:jackett", recursive=True)
        self.create_dir("/var/lib/jackett")
        self.chown("/var/lib/jackett", "jackett:jackett", recursive=True)

        # Create and start systemd service
        self.create_service(
            "jackett",
            exec_start=(
                "/opt/Jackett/jackett"
                " --NoRestart"
                " --NoUpdates"
                f" --Port={webui_port}"
                " --DataFolder=/var/lib/jackett"
            ),
            description="Jackett Indexer Proxy",
            user="jackett",
            working_directory="/opt/Jackett",
        )

        # Wait for WebUI
        self.wait_for_http(f"http://127.0.0.1:{webui_port}", timeout=30)

        self.log.info("Jackett installed successfully")


run(JackettApp)
