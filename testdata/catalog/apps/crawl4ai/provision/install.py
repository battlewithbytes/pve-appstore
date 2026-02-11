"""Crawl4AI — AI-powered web crawler with REST API."""

from appstore import BaseApp, run


class Crawl4AIApp(BaseApp):
    def install(self):
        api_port = self.inputs.integer("api_port", 11235)
        bind_address = self.inputs.string("bind_address", "0.0.0.0")
        max_concurrent = self.inputs.integer("max_concurrent", 5)
        cache_dir = self.inputs.string("cache_dir", "/var/lib/crawl4ai/cache")
        headless = self.inputs.boolean("headless", True)

        # Install system dependencies
        self.pkg_install(
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

        # Install crawl4ai in a venv with API server deps
        self.create_venv("/opt/crawl4ai/venv")
        self.pip_install("crawl4ai", "fastapi", "uvicorn", venv="/opt/crawl4ai/venv")

        # Deploy server and playground files
        self.deploy_provision_file("server.py", "/opt/crawl4ai/server.py")
        self.deploy_provision_file("playground.html", "/opt/crawl4ai/playground.html")

        # Set ownership before installing browsers so they land in the right cache
        self.chown("/opt/crawl4ai", "crawl4ai:crawl4ai", recursive=True)
        self.chown(cache_dir, "crawl4ai:crawl4ai", recursive=True)

        # Install Playwright browsers as the crawl4ai user so they go to its home cache
        self.run_command(["su", "-s", "/bin/bash", "crawl4ai", "-c",
                          "/opt/crawl4ai/venv/bin/playwright install chromium"])

        # Create systemd service
        self.create_service("crawl4ai",
            exec_start="/opt/crawl4ai/venv/bin/python /opt/crawl4ai/server.py",
            description="Crawl4AI Web Crawler API",
            after="network.target",
            user="crawl4ai",
            working_directory="/opt/crawl4ai",
            environment={
                "CRAWL4AI_API_PORT": str(api_port),
                "CRAWL4AI_HOST": bind_address,
                "CRAWL4AI_MAX_CONCURRENT": str(max_concurrent),
                "CRAWL4AI_CACHE_DIR": cache_dir,
                "CRAWL4AI_HEADLESS": str(headless).lower(),
            },
            restart="on-failure",
            restart_sec=5,
        )
        self.log.info("Crawl4AI installed successfully")


run(Crawl4AIApp)
