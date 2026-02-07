"""Jellyfin — free software media system."""

from appstore import BaseApp, run

NETWORK_CONFIG = """\
<?xml version="1.0" encoding="utf-8"?>
<NetworkConfiguration>
  <HttpServerPortNumber>$http_port</HttpServerPortNumber>
</NetworkConfiguration>
"""


class JellyfinApp(BaseApp):
    def install(self):
        media_path = self.inputs.string("media_path", "/mnt/media")
        http_port = self.inputs.string("http_port", "8096")
        cache_path = self.inputs.string("cache_path", "/var/cache/jellyfin")

        self.apt_install("curl", "gnupg")

        # Install Jellyfin via upstream installer
        self.run_installer_script("https://repo.jellyfin.org/install-debuntu.sh")

        # Create media and cache directories
        self.create_dir(media_path)
        self.create_dir(cache_path, owner="jellyfin:jellyfin")

        # Configure custom port if non-default
        if http_port != "8096":
            self.create_dir("/etc/jellyfin")
            self.write_config(
                "/etc/jellyfin/network.xml",
                NETWORK_CONFIG,
                http_port=http_port,
            )
            self.chown("/etc/jellyfin/network.xml", "jellyfin:jellyfin")

        self.enable_service("jellyfin")
        self.log.info("Jellyfin installed successfully")


run(JellyfinApp)
