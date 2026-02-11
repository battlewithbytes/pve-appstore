#!/usr/bin/env python3
"""Resilio Sync — fast peer-to-peer file synchronization."""
import json
from appstore import BaseApp, run

SYNC_CONF_PATH = "/etc/resilio-sync/config.json"


class ResilioSync(BaseApp):
    def install(self):
        # Add Resilio apt repository and install
        self.add_apt_repository(
            "https://linux-packages.resilio.com/resilio-sync/deb",
            key_url="https://linux-packages.resilio.com/resilio-sync/key.asc",
            name="resilio-sync",
            suite="resilio-sync",
            components="non-free",
        )
        self.pkg_install("resilio-sync")

        # Create data directories owned by the rslsync service user
        self.create_dir("/config")
        self.create_dir("/sync")
        self.chown("/config", "rslsync:rslsync", recursive=True)
        self.chown("/sync", "rslsync:rslsync", recursive=True)

        # Write config and start
        self.configure()
        self.enable_service("resilio-sync")
        self.log.info("Resilio Sync installed successfully")

    def configure(self):
        """Write config.json from inputs. Called by install() and reconfigure."""
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        webui_port = self.inputs.integer("webui_port", 8888)
        listening_port = self.inputs.integer("listening_port", 55555)

        # Load existing config to preserve secrets/shares, or start fresh
        try:
            with open(SYNC_CONF_PATH) as f:
                conf = json.loads(f.read())
        except (FileNotFoundError, json.JSONDecodeError):
            conf = {}

        conf["storage_path"] = "/config"
        conf["listening_port"] = listening_port
        conf.setdefault("directory_root", "/sync")
        conf.setdefault("use_upnp", False)
        conf["webui"] = {
            **conf.get("webui", {}),
            "listen": f"{bind_address}:{webui_port}",
        }

        self.write_config(SYNC_CONF_PATH, json.dumps(conf, indent=2) + "\n")
        self.log.info(f"Config written: listen={bind_address}:{webui_port}, sync port={listening_port}")

        # Restart service to pick up new config (no-op on first install)
        self.restart_service("resilio-sync")


run(ResilioSync)
