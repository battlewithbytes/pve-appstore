"""Ollama — local LLM inference server."""

import os

from appstore import BaseApp, run


class OllamaApp(BaseApp):
    def _detect_gpu(self):
        """Detect GPU availability inside LXC by checking device nodes."""
        if os.path.exists("/dev/nvidia0"):
            self.log.info("NVIDIA GPU detected (/dev/nvidia0 present)")
            return "nvidia"
        if os.path.exists("/dev/dri/renderD128"):
            self.log.info("DRI render device detected (/dev/dri/renderD128)")
            return "dri"
        self.log.info("No GPU devices detected — running in CPU-only mode")
        return None

    def install(self):
        api_port = self.inputs.integer("api_port", 11434)
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        models_path = self.inputs.string("models_path", "/usr/share/ollama/.ollama/models")
        num_ctx = self.inputs.integer("num_ctx", 2048)
        default_model = self.inputs.string("model", "")

        # Detect GPU before install
        gpu_type = self._detect_gpu()

        # Install Ollama via upstream installer script
        self.run_installer_script("https://ollama.ai/install.sh")

        # Build systemd override config from template
        override = self.provision_file("systemd-override.conf")
        if gpu_type == "nvidia":
            self.log.info("Configuring NVIDIA GPU environment for Ollama")
            override += self.provision_file("nvidia-env.conf")

        # Configure environment overrides
        self.create_dir("/etc/systemd/system/ollama.service.d")
        self.write_config(
            "/etc/systemd/system/ollama.service.d/override.conf",
            override,
            bind_address=bind_address,
            api_port=api_port,
            models_path=models_path,
            num_ctx=num_ctx,
        )

        # Ensure models directory and parent .ollama dir exist with correct ownership
        self.create_dir(models_path)
        self.chown("/usr/share/ollama/.ollama", "ollama:ollama", recursive=True)

        # Restart with new config
        self.restart_service("ollama")

        # Pull default model if specified
        if default_model:
            api_url = f"http://127.0.0.1:{api_port}"
            if self.wait_for_http(api_url, timeout=60, interval=2):
                self.log.info(f"Pulling model: {default_model}")
                try:
                    self.run_command(["ollama", "pull", default_model])
                except Exception as e:
                    self.log.warn(f"Model pull failed (non-fatal): {e}")
                    self.log.info("You can pull the model manually with: ollama pull " + default_model)
            else:
                self.log.warn("Skipping model pull — Ollama API not ready")
                self.log.info("Pull model manually after service starts: ollama pull " + default_model)

        if gpu_type == "nvidia":
            self.log.info("Ollama installed with NVIDIA GPU support")
        else:
            self.log.info("Ollama installed (CPU mode)")


run(OllamaApp)
