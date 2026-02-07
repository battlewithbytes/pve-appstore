"""Ollama — local LLM inference server."""

from appstore import BaseApp, run

SYSTEMD_OVERRIDE = """\
[Service]
Environment="OLLAMA_HOST=$bind_address:$api_port"
Environment="OLLAMA_MODELS=$models_path"
Environment="OLLAMA_NUM_CTX=$num_ctx"
"""


class OllamaApp(BaseApp):
    def install(self):
        api_port = self.inputs.string("api_port", "11434")
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        models_path = self.inputs.string("models_path", "/usr/share/ollama/.ollama/models")
        num_ctx = self.inputs.string("num_ctx", "2048")
        default_model = self.inputs.string("model", "")

        # Install Ollama via upstream installer script
        self.run_installer_script("https://ollama.ai/install.sh")

        # Configure environment
        self.create_dir("/etc/systemd/system/ollama.service.d")
        self.write_config(
            "/etc/systemd/system/ollama.service.d/override.conf",
            SYSTEMD_OVERRIDE,
            bind_address=bind_address,
            api_port=api_port,
            models_path=models_path,
            num_ctx=num_ctx,
        )

        self.create_dir(models_path)
        self.restart_service("ollama")

        # Pull default model if specified
        if default_model:
            self.log.info(f"Pulling model: {default_model}")
            self.run_command(["ollama", "pull", default_model])

        self.log.info("Ollama installed successfully")


run(OllamaApp)
