"""Hello World — Nginx static page demo app."""

from appstore import BaseApp, run


class HelloWorldApp(BaseApp):
    def install(self):
        self.apt_install("nginx")

        greeting = self.inputs.string("greeting", "Hello from Proxmox!")
        subtitle = self.inputs.string("subtitle", "Your PVE App Store is working correctly.")
        http_port = self.inputs.integer("http_port", 80)
        bg_color = self.inputs.string("bg_color", "#1a1a2e")

        self.render_template("index.html", "/var/www/html/index.html",
            greeting=greeting,
            subtitle=subtitle,
            bg_color=bg_color,
        )

        if http_port != 80:
            self.render_template("default.conf", "/etc/nginx/sites-available/default",
                http_port=http_port,
            )

        self.enable_service("nginx")
        self.log.info("Hello World installed successfully")


run(HelloWorldApp)
