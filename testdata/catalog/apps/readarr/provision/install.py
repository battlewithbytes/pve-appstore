"""Readarr — book and audiobook collection manager."""

from appstore import BaseApp, run


class ReadarrApp(BaseApp):
    def install(self):
        webui_port = self.inputs.string("webui_port", "8787")

        # Install .NET runtime dependencies
        self.apt_install("curl", "sqlite3", "libicu72")

        # Create service user
        self.create_user("readarr", system=True, home="/var/lib/readarr")

        # Download and extract Readarr from Servarr CDN (develop branch — no stable release yet)
        self.download_and_extract(
            "https://readarr.servarr.com/v1/update/develop/updatefile"
            "?os=linux&runtime=netcore&arch=x64",
            "/opt",
        )

        # Symlink system SQLite for native interop
        self.create_symlink(
            "/usr/lib/x86_64-linux-gnu/libsqlite3.so.0",
            "/opt/Readarr/libe_sqlite3.so",
        )

        # Ensure books directory exists (skip chown — may be a bind mount)
        self.create_dir("/books")

        # Set ownership
        self.chown("/opt/Readarr", "readarr:readarr", recursive=True)
        self.chown("/var/lib/readarr", "readarr:readarr", recursive=True)

        # Create and start systemd service
        self.create_service(
            "readarr",
            exec_start=f"/opt/Readarr/Readarr -nobrowser -data=/var/lib/readarr",
            description="Readarr Book Manager",
            user="readarr",
            working_directory="/opt/Readarr",
        )

        # Wait for WebUI
        self.wait_for_http(f"http://127.0.0.1:{webui_port}", timeout=30)

        self.log.info("Readarr installed successfully")


run(ReadarrApp)
