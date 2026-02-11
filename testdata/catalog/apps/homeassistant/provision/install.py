"""Home Assistant — open source home automation."""

from appstore import BaseApp, run

HA_CONFIG = """\
homeassistant:
  name: Home
  time_zone: $timezone

http:
  server_port: $http_port
"""

MQTT_HA_CONFIG = """\

mqtt:
  broker: 127.0.0.1
  port: 1883
"""


class HomeAssistantApp(BaseApp):
    def install(self):
        timezone = self.inputs.string("timezone", "America/New_York")
        http_port = self.inputs.string("http_port", "8123")
        config_path = self.inputs.string("config_path", "/opt/homeassistant/config")
        enable_mqtt = self.inputs.boolean("enable_mqtt", False)

        # Install system dependencies
        self.apt_install(
            "python3", "python3-venv", "python3-pip",
            "libffi-dev", "libssl-dev", "libjpeg-dev",
            "zlib1g-dev", "autoconf", "build-essential",
            "libopenjp2-7", "libtiff6",
        )

        # Set container timezone
        self.run_command(["ln", "-sf", f"/usr/share/zoneinfo/{timezone}", "/etc/localtime"])
        self.write_config("/etc/timezone", timezone + "\n")
        self.run_command(["dpkg-reconfigure", "-f", "noninteractive", "tzdata"])

        # Create app user and directories
        self.create_user("homeassistant", system=True, home="/opt/homeassistant")
        self.create_dir(config_path)

        # Install Home Assistant in a venv
        self.create_venv("/opt/homeassistant/venv")
        self.pip_install("homeassistant", venv="/opt/homeassistant/venv")

        # Write Home Assistant configuration
        self.write_config(
            f"{config_path}/configuration.yaml",
            HA_CONFIG,
            timezone=timezone,
            http_port=http_port,
        )

        # Install MQTT broker if requested
        if enable_mqtt:
            self.apt_install("mosquitto", "mosquitto-clients")
            self.enable_service("mosquitto")
            # Append MQTT config to HA configuration
            with open(f"{config_path}/configuration.yaml", "a") as f:
                f.write(MQTT_HA_CONFIG)
            self.log.info("MQTT broker installed and running on port 1883")

        # Set ownership
        self.chown("/opt/homeassistant", "homeassistant:homeassistant", recursive=True)
        self.chown(config_path, "homeassistant:homeassistant", recursive=True)

        # Create systemd service
        self.create_service("homeassistant",
            exec_start=f"/opt/homeassistant/venv/bin/hass -c {config_path}",
            description="Home Assistant Core",
            user="homeassistant",
            working_directory="/opt/homeassistant",
            environment={"PATH": "/opt/homeassistant/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
            restart="on-failure",
            restart_sec=10,
        )
        self.log.info("Home Assistant installed successfully")


run(HomeAssistantApp)
