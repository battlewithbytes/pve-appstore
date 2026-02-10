"""Hello World — Nginx static page demo app."""

from appstore import BaseApp, run

INDEX_TEMPLATE = """\
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>$greeting</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background: $bg_color;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      color: #fff;
    }
    .container { text-align: center; padding: 48px; }
    .icon { font-size: 64px; margin-bottom: 24px; }
    h1 { font-size: 2.5rem; font-weight: 700; margin-bottom: 12px; }
    p { font-size: 1.1rem; color: #aaa; margin-bottom: 32px; }
    .badge {
      display: inline-block; padding: 6px 16px;
      background: rgba(255,255,255,0.1); border-radius: 20px;
      font-size: 0.85rem; color: #7ec8e3;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="icon">&#9881;</div>
    <h1>$greeting</h1>
    <p>$subtitle</p>
    <span class="badge">PVE App Store</span>
  </div>
</body>
</html>
"""

NGINX_CONF_TEMPLATE = """\
server {
    listen $http_port default_server;
    listen [::]:$http_port default_server;
    root /var/www/html;
    index index.html;
    location / {
        try_files $$uri $$uri/ =404;
    }
}
"""


class HelloWorldApp(BaseApp):
    def install(self):
        self.apt_install("nginx")

        greeting = self.inputs.string("greeting", "Hello from Proxmox!")
        subtitle = self.inputs.string("subtitle", "Your PVE App Store is working correctly.")
        http_port = self.inputs.string("http_port", "80")
        bg_color = self.inputs.string("bg_color", "#1a1a2e")

        self.write_config(
            "/var/www/html/index.html",
            INDEX_TEMPLATE,
            greeting=greeting,
            subtitle=subtitle,
            bg_color=bg_color,
        )

        if http_port != "80":
            self.write_config(
                "/etc/nginx/sites-available/default",
                NGINX_CONF_TEMPLATE,
                http_port=http_port,
            )

        self.enable_service("nginx")
        self.log.info("Hello World installed successfully")


run(HelloWorldApp)
