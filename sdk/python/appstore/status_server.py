#!/usr/bin/env python3
"""Standalone status page server deployed into containers by BaseApp.status_page().

Reads configuration from status_config.json in the same directory:
    {
        "port": 8001,
        "title": "App Name",
        "api_url": "http://127.0.0.1:8000/api",
        "fields": {"key": "Label", ...},
        "bind_lan_only": true
    }

Serves a single-page status dashboard with the CCO dark theme.
Polls the configured API URL and renders field values in a grid.
"""

import fcntl
import html
import http.server
import json
import os
import socket
import struct
import urllib.request

# Load config from JSON file next to this script
_config_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "status_config.json")
with open(_config_path) as _f:
    CONFIG = json.load(_f)

PORT = CONFIG["port"]
TITLE = CONFIG["title"]
API_URL = CONFIG["api_url"]
FIELDS = CONFIG["fields"]  # {"api_key": "Display Label", ...}
BIND_LAN_ONLY = CONFIG.get("bind_lan_only", True)

CSS = """\
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0a0a0a;color:#e0e0e0;font-family:"Inter","Segoe UI",system-ui,sans-serif;
     display:flex;justify-content:center;align-items:center;min-height:100vh;padding:20px}
.card{background:#141414;border:1px solid #222;border-radius:16px;padding:40px;
      max-width:480px;width:100%;box-shadow:0 4px 24px rgba(0,0,0,0.4)}
h1{font-size:1.4rem;margin-bottom:24px;display:flex;align-items:center;gap:10px}
.dot{width:12px;height:12px;border-radius:50%;display:inline-block}
.dot.up{background:#00ff9d;box-shadow:0 0 8px #00ff9d80}
.dot.down{background:#ff4444;box-shadow:0 0 8px #ff444480}
.label{font-size:.75rem;text-transform:uppercase;letter-spacing:.08em;color:#888;margin-bottom:4px}
.value{font-size:1.1rem;font-family:"JetBrains Mono","Fira Code",monospace;color:#fff;
       margin-bottom:16px;word-break:break-all}
.value.accent{color:#00ff9d}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:20px}
.grid .value{font-size:1rem;margin-bottom:0}
.footer{font-size:.7rem;color:#555;text-align:center;margin-top:24px;padding-top:16px;
        border-top:1px solid #222}
.error{color:#ff4444;font-size:.9rem;padding:16px;background:#1a0a0a;border-radius:8px;
       border:1px solid #331111}
.refresh{font-size:.75rem;color:#00ff9d;text-decoration:none;float:right;margin-top:-32px}
.refresh:hover{text-decoration:underline}
"""


def get_lan_ip(ifname="eth0"):
    """Get the IP of the LAN interface so we only bind to the local network."""
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        return socket.inet_ntoa(fcntl.ioctl(
            s.fileno(), 0x8915,  # SIOCGIFADDR
            struct.pack("256s", ifname[:15].encode()),
        )[20:24])
    except Exception:
        return "127.0.0.1"


def fetch_status():
    """Poll the configured API URL and return parsed data."""
    try:
        resp = urllib.request.urlopen(API_URL, timeout=5)
        return {"ok": True, "data": json.loads(resp.read())}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def render_page(status):
    """Render the HTML status page."""
    if status["ok"]:
        data = status["data"]
        # Build grid items from configured fields
        items = []
        first = True
        for key, label in FIELDS.items():
            val = html.escape(str(data.get(key, "N/A")))
            css = "value accent" if first else "value"
            if first:
                # First field gets prominent display
                items.append(
                    f'<div class="label">{html.escape(label)}</div>'
                    f'<div class="{css}">{val}</div>'
                )
                first = False
            else:
                items.append(
                    f'<div><div class="label">{html.escape(label)}</div>'
                    f'<div class="{css}">{val}</div></div>'
                )

        # First field is outside grid, rest inside
        if len(items) > 1:
            content = items[0] + '<div class="grid">' + "".join(items[1:]) + "</div>"
        else:
            content = items[0] if items else ""
        dot = "up"
    else:
        content = (
            f'<div class="error">{html.escape(status["error"])}</div>'
        )
        dot = "down"

    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{html.escape(TITLE)}</title>
<style>{CSS}</style>
</head>
<body>
<div class="card">
  <h1><span class="dot {dot}"></span> {html.escape(TITLE)}</h1>
  <a href="/" class="refresh">Refresh</a>
  {content}
  <div class="footer">{html.escape(TITLE)} &middot; Status Page</div>
</div>
</body>
</html>"""


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        page = render_page(fetch_status())
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(page.encode())

    def log_message(self, format, *args):
        pass  # suppress request logging


if __name__ == "__main__":
    bind_ip = get_lan_ip() if BIND_LAN_ONLY else "0.0.0.0"
    server = http.server.HTTPServer((bind_ip, PORT), Handler)
    print(f"Status page listening on {bind_ip}:{PORT}")
    server.serve_forever()
