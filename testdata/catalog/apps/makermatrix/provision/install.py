"""MakerMatrix — electronic parts inventory manager for makers."""

from appstore import BaseApp, run


class MakerMatrixApp(BaseApp):
    def install(self):
        web_port = self.inputs.string("web_port", "8080")
        jwt_secret = self.inputs.string("jwt_secret", "")
        if not jwt_secret:
            import secrets
            jwt_secret = secrets.token_hex(32)

        # 1. System dependencies
        self.log.progress(1, 8, "Installing system packages")
        self.apt_install(
            "python3", "python3-venv", "python3-pip", "python3-dev",
            "nodejs", "npm",
            "gcc", "g++", "zlib1g-dev", "fonts-dejavu-core",
            "git", "curl",
        )

        # 2. App user and directories
        self.log.progress(2, 8, "Creating app user")
        self.create_user("makermatrix", system=True, home="/opt/makermatrix")

        # 3. Clone repository
        self.log.progress(3, 8, "Cloning MakerMatrix repository")
        self.run_command([
            "git", "clone", "--depth=1",
            "https://github.com/ril3y/MakerMatrix.git",
            "/opt/makermatrix/app",
        ])

        # 4. Python virtual environment + dependencies
        self.log.progress(4, 8, "Installing Python dependencies")
        self.create_venv("/opt/makermatrix/venv")
        self.pip_install(
            "-r", "/opt/makermatrix/app/requirements.txt",
            venv="/opt/makermatrix/venv",
        )

        # 5. Build React frontend
        self.log.progress(5, 8, "Building frontend")
        self.run_command(
            ["npm", "ci"],
            cwd="/opt/makermatrix/app/MakerMatrix/frontend",
        )
        self.run_command(
            ["npx", "vite", "build"],
            cwd="/opt/makermatrix/app/MakerMatrix/frontend",
        )

        # 6. Data directories
        self.log.progress(6, 8, "Setting up data directories")
        for d in ["database", "static", "static/datasheets", "static/images", "backups", "certs"]:
            self.create_dir(f"/data/{d}")

        # 7. Environment file
        self.write_env_file("/opt/makermatrix/env", {
            "DATABASE_URL": "sqlite:////data/database/makermatrix.db",
            "STATIC_FILES_PATH": "/data/static",
            "BACKUPS_PATH": "/data/backups",
            "CERTS_PATH": "/data/certs",
            "JWT_SECRET_KEY": jwt_secret,
            "HOST": "0.0.0.0",
            "PORT": web_port,
            "DEBUG": "false",
            "LOG_LEVEL": "INFO",
        })

        # 8. Set ownership
        self.chown("/opt/makermatrix", "makermatrix:makermatrix", recursive=True)
        self.chown("/data", "makermatrix:makermatrix", recursive=True)

        # 9. Systemd service
        self.log.progress(7, 8, "Creating systemd service")
        self.create_service(
            "makermatrix",
            exec_start="/opt/makermatrix/venv/bin/python -m MakerMatrix.main",
            description="MakerMatrix Inventory Manager",
            user="makermatrix",
            working_directory="/opt/makermatrix/app",
            environment_file="/opt/makermatrix/env",
        )

        # 10. Wait for health check
        self.log.progress(8, 8, "Waiting for MakerMatrix to start")
        self.wait_for_http(
            f"http://127.0.0.1:{web_port}/api/utility/get_counts",
            timeout=60,
        )

        self.log.info("MakerMatrix installed successfully")
        self.log.output("default_user", "admin")
        self.log.output("default_password", "Admin123!")


run(MakerMatrixApp)
