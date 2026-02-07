"""Crawl4AI — AI-powered web crawler with REST API."""

from appstore import BaseApp, run

SYSTEMD_UNIT = """\
[Unit]
Description=Crawl4AI Web Crawler API
After=network.target

[Service]
Type=simple
User=crawl4ai
Environment="CRAWL4AI_API_PORT=$api_port"
Environment="CRAWL4AI_HOST=$bind_address"
Environment="CRAWL4AI_MAX_CONCURRENT=$max_concurrent"
Environment="CRAWL4AI_CACHE_DIR=$cache_dir"
Environment="CRAWL4AI_HEADLESS=$headless"
ExecStart=/opt/crawl4ai/venv/bin/python -m crawl4ai.server --host $bind_address --port $api_port
Restart=on-failure
RestartSec=5
WorkingDirectory=/opt/crawl4ai

[Install]
WantedBy=multi-user.target
"""


class Crawl4AIApp(BaseApp):
    def install(self):
        api_port = self.inputs.string("api_port", "11235")
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        max_concurrent = self.inputs.string("max_concurrent", "5")
        cache_dir = self.inputs.string("cache_dir", "/var/lib/crawl4ai/cache")
        headless = self.inputs.string("headless", "true")

        # Install system dependencies
        self.apt_install(
            "python3", "python3-venv", "python3-pip",
            "curl", "wget", "gnupg",
            "libnss3", "libnspr4", "libatk1.0-0", "libatk-bridge2.0-0",
            "libcups2", "libdrm2", "libxkbcommon0", "libxcomposite1",
            "libxdamage1", "libxfixes3", "libxrandr2", "libgbm1",
            "libpango-1.0-0", "libcairo2", "libasound2", "libatspi2.0-0",
        )

        # Create app user and directories
        self.create_user("crawl4ai", system=True, home="/opt/crawl4ai")
        self.create_dir(cache_dir)
        self.create_dir("/opt/crawl4ai")

        # Install crawl4ai in a venv
        self.create_venv("/opt/crawl4ai/venv")
        self.pip_install("crawl4ai", venv="/opt/crawl4ai/venv")

        # Install Playwright browsers
        self.run_command(["/opt/crawl4ai/venv/bin/crawl4ai-setup"])

        self.chown("/opt/crawl4ai", "crawl4ai:crawl4ai", recursive=True)
        self.chown(cache_dir, "crawl4ai:crawl4ai", recursive=True)

        # Create systemd service
        self.write_config(
            "/etc/systemd/system/crawl4ai.service",
            SYSTEMD_UNIT,
            api_port=api_port,
            bind_address=bind_address,
            max_concurrent=max_concurrent,
            cache_dir=cache_dir,
            headless=headless,
        )

        self.enable_service("crawl4ai")
        self.log.info("Crawl4AI installed successfully")


run(Crawl4AIApp)
