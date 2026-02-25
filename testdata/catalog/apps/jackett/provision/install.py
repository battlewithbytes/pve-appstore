"""Jackett — API proxy server for torrent trackers."""

import json

from appstore import BaseApp, run


class JackettApp(BaseApp):
    def install(self):
        webui_port = self.inputs.string("webui_port", "9117")

        # Install .NET runtime dependencies
        self.apt_install("curl", "libicu72", "libssl3")

        # Create service user
        self.create_user("jackett", system=True, home="/var/lib/jackett")

        # Get latest release URL from GitHub API
        self.log.info("Fetching latest Jackett release...")
        result = self.run_command([
            "curl", "-fsSL",
            "https://api.github.com/repos/Jackett/Jackett/releases/latest",
        ])
        release = json.loads(result.stdout)
        tag = release["tag_name"]
        self.log.info(f"Latest Jackett version: {tag}")

        # Find the Linux AMD64 tarball asset
        dl_url = None
        for asset in release["assets"]:
            if "LinuxAMDx64" in asset["name"] and asset["name"].endswith(".tar.gz"):
                dl_url = asset["browser_download_url"]
                break

        if not dl_url:
            raise RuntimeError("Could not find Jackett Linux AMD64 release asset")

        # Download and extract
        self.download(dl_url, "/tmp/jackett.tar.gz")
        self.create_dir("/opt/Jackett")
        self.run_command(["tar", "-xzf", "/tmp/jackett.tar.gz", "-C", "/opt"])

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
