"""Audiobookshelf — self-hosted audiobook and podcast server."""

from appstore import BaseApp, run


class AudiobookshelfApp(BaseApp):
    def install(self):
        port = self.inputs.string("port", "13378")

        # Install prerequisites for APT key management
        self.apt_install("gnupg", "curl")

        # Add Audiobookshelf APT repository
        self.add_apt_key("https://advplyr.github.io/audiobookshelf-ppa/KEY.gpg")
        self.add_apt_repo(
            "audiobookshelf",
            "deb https://advplyr.github.io/audiobookshelf-ppa ./",
        )

        # Install audiobookshelf from PPA
        self.apt_install("audiobookshelf")

        # Ensure media directories exist
        self.create_dir("/audiobooks")
        self.create_dir("/podcasts")

        # Configure audiobookshelf defaults
        self.write_env_file("/etc/default/audiobookshelf", {
            "PORT": port,
            "CONFIG_PATH": "/usr/share/audiobookshelf/config",
            "METADATA_PATH": "/usr/share/audiobookshelf/metadata",
        })

        # The package installs its own systemd service — restart with new config
        self.restart_service("audiobookshelf")

        # Wait for server to start
        self.wait_for_http(f"http://127.0.0.1:{port}", timeout=30)

        self.log.info("Audiobookshelf installed successfully")


run(AudiobookshelfApp)
