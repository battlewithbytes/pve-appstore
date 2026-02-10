"""Jellyfin — free software media system."""

from appstore import BaseApp, run

NETWORK_CONFIG = """\
<?xml version="1.0" encoding="utf-8"?>
<NetworkConfiguration>
  <HttpServerPortNumber>$http_port</HttpServerPortNumber>
</NetworkConfiguration>
"""

ENCODING_CONFIG_QSV = """\
<?xml version="1.0" encoding="utf-8"?>
<EncodingOptions>
  <HardwareAccelerationType>vaapi</HardwareAccelerationType>
  <VaapiDevice>/dev/dri/renderD128</VaapiDevice>
  <EnableHardwareEncoding>true</EnableHardwareEncoding>
  <EnableTonemapping>true</EnableTonemapping>
</EncodingOptions>
"""

ENCODING_CONFIG_NVENC = """\
<?xml version="1.0" encoding="utf-8"?>
<EncodingOptions>
  <HardwareAccelerationType>nvenc</HardwareAccelerationType>
  <EnableHardwareEncoding>true</EnableHardwareEncoding>
  <EnableTonemapping>true</EnableTonemapping>
</EncodingOptions>
"""

SYSTEMD_OVERRIDE = """\
[Service]
Environment="JELLYFIN_CACHE_DIR=$cache_path"
"""


class JellyfinApp(BaseApp):
    def install(self):
        media_path = self.inputs.string("media_path", "/mnt/media")
        http_port = self.inputs.string("http_port", "8096")
        cache_path = self.inputs.string("cache_path", "/var/cache/jellyfin")
        transcode_threads = self.inputs.string("transcode_threads", "0")
        hw_accel = self.inputs.string("hw_accel", "none")

        # Install system dependencies
        self.apt_install("curl", "gnupg")

        # Install Jellyfin via upstream installer
        self.run_installer_script("https://repo.jellyfin.org/install-debuntu.sh")

        # Create media and cache directories
        self.create_dir(media_path)
        self.create_dir(cache_path, owner="jellyfin:jellyfin")

        # Jellyfin config directory
        config_dir = "/etc/jellyfin"
        self.create_dir(config_dir)

        # Configure custom port if non-default
        if http_port != "8096":
            self.write_config(
                f"{config_dir}/network.xml",
                NETWORK_CONFIG,
                http_port=http_port,
            )
            self.chown(f"{config_dir}/network.xml", "jellyfin:jellyfin")

        # Configure hardware acceleration
        if hw_accel == "qsv":
            self.write_config(f"{config_dir}/encoding.xml", ENCODING_CONFIG_QSV)
            self.chown(f"{config_dir}/encoding.xml", "jellyfin:jellyfin")
            # Ensure jellyfin user can access render device
            self.run_command(["usermod", "-aG", "render", "jellyfin"])
            self.run_command(["usermod", "-aG", "video", "jellyfin"])
            self.log.info("Intel QSV hardware acceleration configured")
        elif hw_accel == "nvenc":
            self.write_config(f"{config_dir}/encoding.xml", ENCODING_CONFIG_NVENC)
            self.chown(f"{config_dir}/encoding.xml", "jellyfin:jellyfin")
            self.log.info("NVIDIA NVENC hardware acceleration configured")

        # Configure cache directory override
        if cache_path != "/var/cache/jellyfin":
            self.create_dir("/etc/systemd/system/jellyfin.service.d")
            self.write_config(
                "/etc/systemd/system/jellyfin.service.d/override.conf",
                SYSTEMD_OVERRIDE,
                cache_path=cache_path,
            )

        self.enable_service("jellyfin")
        self.log.info("Jellyfin installed successfully")


run(JellyfinApp)
