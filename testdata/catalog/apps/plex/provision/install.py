"""Plex Media Server — personal media streaming."""

from appstore import BaseApp, run

PREFS_TEMPLATE = """\
<?xml version="1.0" encoding="utf-8"?>
<Preferences FriendlyName="$friendly_name" ManualPortMappingPort="$http_port" TranscoderTempDirectory="$transcode_path"/>
"""


class PlexApp(BaseApp):
    def install(self):
        media_path = self.inputs.string("media_path", "/mnt/media")
        transcode_path = self.inputs.string("transcode_path", "/tmp/plex-transcode")
        http_port = self.inputs.string("http_port", "32400")
        friendly_name = self.inputs.string("friendly_name", "Proxmox Plex")

        self.apt_install("curl")

        # Add Plex APT key and repository
        self.add_apt_key(
            "https://downloads.plex.tv/plex-keys/PlexSign.key",
            "/usr/share/keyrings/plex-archive-keyring.gpg",
        )
        self.add_apt_repo(
            "deb [signed-by=/usr/share/keyrings/plex-archive-keyring.gpg] https://downloads.plex.tv/repo/deb public main",
            "plexmediaserver.list",
        )

        self.apt_install("plexmediaserver")

        # Create directories
        self.create_dir(media_path)
        self.create_dir(transcode_path, owner="plex:plex")

        # Write Plex preferences
        prefs_dir = "/var/lib/plexmediaserver/Library/Application Support/Plex Media Server"
        self.create_dir(prefs_dir)
        self.write_config(
            f"{prefs_dir}/Preferences.xml",
            PREFS_TEMPLATE,
            friendly_name=friendly_name,
            http_port=http_port,
            transcode_path=transcode_path,
        )
        self.chown(f"{prefs_dir}/Preferences.xml", "plex:plex")

        self.enable_service("plexmediaserver")
        self.log.info("Plex Media Server installed successfully")


run(PlexApp)
