"""Nginx — high-performance HTTP server and reverse proxy."""

from appstore import BaseApp, run


class NginxApp(BaseApp):
    def install(self):
        self.apt_install("nginx")

        domain = self.inputs.string("domain", "")
        enable_ssl = self.inputs.boolean("enable_ssl", False)
        http_port = self.inputs.integer("http_port", 80)
        https_port = self.inputs.integer("https_port", 443)
        worker_processes = self.inputs.integer("worker_processes", 0)

        server_name_line = f"server_name {domain};" if domain else ""

        # Configure worker processes
        if worker_processes != 0:
            self.run_command([
                "sed", "-i",
                f"s/worker_processes auto;/worker_processes {worker_processes};/",
                "/etc/nginx/nginx.conf",
            ])

        # Write default server block
        self.render_template("server.conf", "/etc/nginx/sites-available/default",
            http_port=http_port,
            server_name_line=server_name_line,
        )

        # Generate self-signed SSL if requested
        if enable_ssl:
            self.create_dir("/etc/nginx/ssl")
            cn = domain if domain else "localhost"
            self.run_command([
                "openssl", "req", "-x509", "-nodes", "-days", "365",
                "-newkey", "rsa:2048",
                "-keyout", "/etc/nginx/ssl/nginx.key",
                "-out", "/etc/nginx/ssl/nginx.crt",
                "-subj", f"/CN={cn}",
            ])

            # Append SSL server block
            ssl_content = self.provision_file("ssl.conf")
            ssl_content = ssl_content.replace("$https_port", str(https_port))
            ssl_content = ssl_content.replace("$server_name_line", server_name_line)
            with open("/etc/nginx/sites-available/default", "a") as f:
                f.write(ssl_content)

        self.enable_service("nginx")
        self.log.info("Nginx installed successfully")


run(NginxApp)
