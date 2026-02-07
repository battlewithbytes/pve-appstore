"""Home Assistant — open source home automation."""

from appstore import BaseApp, run

HA_CONFIG = """\
http:
  server_port: $http_port
"""


class HomeAssistantApp(BaseApp):
    def install(self):
        http_port = self.inputs.string("http_port", "8123")
        config_path = self.inputs.string("config_path", "/opt/homeassistant/config")
        enable_mqtt = self.inputs.boolean("enable_mqtt", False)

        self.apt_install("python3", "python3-venv", "python3-pip")

        self.create_venv("/opt/homeassistant")
        self.pip_install("homeassistant", venv="/opt/homeassistant")

        self.create_dir(config_path)

        # Configure custom port if non-default
        if http_port != "8123":
            self.write_config(
                f"{config_path}/configuration.yaml",
                HA_CONFIG,
                http_port=http_port,
            )

        # Install MQTT broker if requested
        if enable_mqtt:
            self.apt_install("mosquitto", "mosquitto-clients")
            self.enable_service("mosquitto")
            self.log.info("MQTT broker installed and running on port 1883")

        self.log.info("Home Assistant installed successfully")


run(HomeAssistantApp)
