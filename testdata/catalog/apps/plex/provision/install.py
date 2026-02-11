"""Plex Media Server — personal media streaming."""

from appstore import BaseApp, run


class PlexApp(BaseApp):
    def install(self):
        media_path = self.inputs.string("media_path", "/mnt/media")
        transcode_path = self.inputs.string("transcode_path", "/tmp/plex-transcode")
        http_port = self.inputs.integer("http_port", 32400)
        friendly_name = self.inputs.string("friendly_name", "Proxmox Plex")
        claim_token = self.inputs.string("claim_token", "")

        # Add Plex APT key and repository
        self.add_apt_repository(
            "https://downloads.plex.tv/repo/deb",
            key_url="https://downloads.plex.tv/plex-keys/PlexSign.key",
            name="plexmediaserver",
            suite="public",
        )

        self.apt_install("plexmediaserver")

        # Create directories
        self.create_dir(media_path)
        self.create_dir(transcode_path, owner="plex:plex")

        # Build claim attribute for Preferences.xml
        claim_attr = ""
        if claim_token:
            claim_attr = f' ProcessedMachineIdentifier="" PlexOnlineToken="{claim_token}"'
            self.log.info("Claim token provided — server will be linked to your Plex account")

        # Write Plex preferences from template
        prefs_dir = "/var/lib/plexmediaserver/Library/Application Support/Plex Media Server"
        self.create_dir(prefs_dir)
        self.render_template("Preferences.xml", f"{prefs_dir}/Preferences.xml",
            friendly_name=friendly_name,
            http_port=http_port,
            transcode_path=transcode_path,
            claim_attr=claim_attr,
        )
        self.chown("/var/lib/plexmediaserver", "plex:plex", recursive=True)

        self.enable_service("plexmediaserver")
        self.log.info("Plex Media Server installed successfully")


run(PlexApp)
