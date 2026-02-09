"""Ollama — local LLM inference server."""

import os
import time
import urllib.request

from appstore import BaseApp, run

SYSTEMD_OVERRIDE = """\
[Service]
Environment="OLLAMA_HOST=$bind_address:$api_port"
Environment="OLLAMA_MODELS=$models_path"
Environment="OLLAMA_NUM_CTX=$num_ctx"
"""

# Additional env vars for NVIDIA GPU support in LXC
SYSTEMD_NVIDIA_SNIPPET = """\
Environment="NVIDIA_VISIBLE_DEVICES=all"
Environment="NVIDIA_DRIVER_CAPABILITIES=compute,utility"
Environment="LD_LIBRARY_PATH=/usr/lib/nvidia"
"""


class OllamaApp(BaseApp):
    def _detect_gpu(self):
        """Detect GPU availability inside LXC by checking device nodes.

        lspci does not work in LXC containers (no PCI bus), so we check
        for device nodes that the engine bind-mounts from the host.
        """
        if os.path.exists("/dev/nvidia0"):
            self.log.info("NVIDIA GPU detected (/dev/nvidia0 present)")
            return "nvidia"
        if os.path.exists("/dev/dri/renderD128"):
            self.log.info("DRI render device detected (/dev/dri/renderD128)")
            return "dri"
        self.log.info("No GPU devices detected — running in CPU-only mode")
        return None

    def _wait_for_service(self, bind_address, api_port, timeout=60):
        """Wait for Ollama API to become ready."""
        api_url = f"http://127.0.0.1:{api_port}"
        self.log.info(f"Waiting for Ollama API at {api_url}...")
        for i in range(timeout // 2):
            try:
                urllib.request.urlopen(api_url, timeout=2)
                self.log.info("Ollama API is ready")
                return True
            except Exception:
                time.sleep(2)
        self.log.warn("Ollama API did not become ready within timeout")
        return False

    def install(self):
        api_port = self.inputs.string("api_port", "11434")
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        models_path = self.inputs.string("models_path", "/usr/share/ollama/.ollama/models")
        num_ctx = self.inputs.string("num_ctx", "2048")
        default_model = self.inputs.string("model", "")

        # Detect GPU before install
        gpu_type = self._detect_gpu()

        # Install Ollama via upstream installer script
        # The script may warn about missing GPU since lspci doesn't work in LXC,
        # but the ollama binary detects GPUs at runtime via NVIDIA libraries.
        self.run_installer_script("https://ollama.ai/install.sh")

        # Build systemd override config
        override = SYSTEMD_OVERRIDE
        if gpu_type == "nvidia":
            self.log.info("Configuring NVIDIA GPU environment for Ollama")
            override += SYSTEMD_NVIDIA_SNIPPET

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
        # Ollama needs write access to .ollama/ for its key file (id_ed25519)
        self.create_dir(models_path)
        self.chown("/usr/share/ollama/.ollama", "ollama:ollama", recursive=True)

        # Restart with new config (restart_service handles daemon-reload)
        self.restart_service("ollama")

        # Pull default model if specified
        if default_model:
            if self._wait_for_service(bind_address, api_port):
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
