"""Nginx — high-performance HTTP server and reverse proxy."""

from appstore import BaseApp, run

SERVER_BLOCK = """\
server {
    listen $http_port default_server;
    listen [::]:$http_port default_server;
    $server_name_line

    root /var/www/html;
    index index.html index.htm;

    location / {
        try_files $$uri $$uri/ =404;
    }
}
"""

SSL_BLOCK = """\

server {
    listen $https_port ssl default_server;
    listen [::]:$https_port ssl default_server;
    $server_name_line

    ssl_certificate /etc/nginx/ssl/nginx.crt;
    ssl_certificate_key /etc/nginx/ssl/nginx.key;

    root /var/www/html;
    index index.html index.htm;

    location / {
        try_files $$uri $$uri/ =404;
    }
}
"""


class NginxApp(BaseApp):
    def install(self):
        self.apt_install("nginx")

        domain = self.inputs.string("domain", "")
        enable_ssl = self.inputs.boolean("enable_ssl", False)
        http_port = self.inputs.string("http_port", "80")
        https_port = self.inputs.string("https_port", "443")
        worker_processes = self.inputs.string("worker_processes", "0")

        server_name_line = f"server_name {domain};" if domain else ""

        # Configure worker processes
        if worker_processes != "0":
            self.run_command([
                "sed", "-i",
                f"s/worker_processes auto;/worker_processes {worker_processes};/",
                "/etc/nginx/nginx.conf",
            ])

        # Write default server block
        self.write_config(
            "/etc/nginx/sites-available/default",
            SERVER_BLOCK,
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
            ssl_content = SSL_BLOCK.replace("$https_port", https_port).replace(
                "$server_name_line", server_name_line
            )
            with open("/etc/nginx/sites-available/default", "a") as f:
                f.write(ssl_content)

        self.enable_service("nginx")
        self.log.info("Nginx installed successfully")


run(NginxApp)
