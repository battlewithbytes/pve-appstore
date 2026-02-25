"""Prowlarr — indexer manager for *arr applications."""

from appstore import BaseApp, run


class ProwlarrApp(BaseApp):
    def install(self):
        webui_port = self.inputs.string("webui_port", "9696")

        # Install .NET runtime dependencies
        self.apt_install("curl", "sqlite3", "libicu72")

        # Create service user
        self.create_user("prowlarr", system=True, home="/var/lib/prowlarr")

        # Download Prowlarr from Servarr CDN
        self.download(
            "https://prowlarr.servarr.com/v1/update/master/updatefile"
            "?os=linux&runtime=netcore&arch=x64",
            "/tmp/prowlarr.tar.gz",
        )

        # Extract to /opt
        self.create_dir("/opt/Prowlarr")
        self.run_command(["tar", "-xzf", "/tmp/prowlarr.tar.gz", "-C", "/opt"])

        # Symlink system SQLite for native interop
        self.run_command([
            "ln", "-sf",
            "/usr/lib/x86_64-linux-gnu/libsqlite3.so.0",
            "/opt/Prowlarr/libe_sqlite3.so",
        ])

        # Set ownership
        self.chown("/opt/Prowlarr", "prowlarr:prowlarr", recursive=True)
        self.chown("/var/lib/prowlarr", "prowlarr:prowlarr", recursive=True)

        # Create and start systemd service
        self.create_service(
            "prowlarr",
            exec_start=f"/opt/Prowlarr/Prowlarr -nobrowser -data=/var/lib/prowlarr",
            description="Prowlarr Indexer Manager",
            user="prowlarr",
            working_directory="/opt/Prowlarr",
        )

        # Wait for WebUI
        self.wait_for_http(f"http://127.0.0.1:{webui_port}", timeout=30)

        self.log.info("Prowlarr installed successfully")


run(ProwlarrApp)
