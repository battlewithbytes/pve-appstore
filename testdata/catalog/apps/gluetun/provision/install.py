"""Gluetun VPN Client — multi-provider VPN with proxy servers and kill switch.

Downloads the Gluetun binary directly from Docker Hub's OCI registry
(no Docker required) and runs it as a systemd service in a privileged
LXC container.
"""

import io
import json
import os
import platform
import tarfile
import time
import urllib.request

from appstore import BaseApp, run

GLUETUN_IMAGE = "qmcgaw/gluetun"
GLUETUN_TAG = "latest"
BINARY_PATH = "/gluetun-entrypoint"

SYSTEMD_UNIT = """\
[Unit]
Description=Gluetun VPN Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/etc/gluetun/start.sh
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
"""

START_SCRIPT = """\
#!/bin/bash
set -a
source /etc/gluetun/env
exec /gluetun-entrypoint
"""

STATUS_SYSTEMD_UNIT = """\
[Unit]
Description=Gluetun VPN Status Page
After=gluetun.service
Requires=gluetun.service

[Service]
Type=simple
ExecStart=/usr/bin/python3 /etc/gluetun/status.py
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
"""

STATUS_PAGE_PY = """\
import http.server
import json
import urllib.request
import html
import socket
import fcntl
import struct
import time

PORT = $status_port

GLUETUN_API = "http://127.0.0.1:8000"


def get_lan_ip(ifname="eth0"):
    \"\"\"Get the IP of the LAN interface (eth0) so we only bind to the local network.\"\"\"
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        return socket.inet_ntoa(fcntl.ioctl(
            s.fileno(), 0x8915,  # SIOCGIFADDR
            struct.pack("256s", ifname[:15].encode()),
        )[20:24])
    except Exception:
        # Fallback to localhost — never bind to 0.0.0.0
        return "127.0.0.1"

CSS = '''
  *{{box-sizing:border-box;margin:0;padding:0}}
  body{{background:#0a0a0a;color:#e0e0e0;font-family:"Inter","Segoe UI",system-ui,sans-serif;
       display:flex;justify-content:center;align-items:center;min-height:100vh;padding:20px}}
  .card{{background:#141414;border:1px solid #222;border-radius:16px;padding:40px;
        max-width:480px;width:100%;box-shadow:0 4px 24px rgba(0,0,0,0.4)}}
  h1{{font-size:1.4rem;margin-bottom:24px;display:flex;align-items:center;gap:10px}}
  .dot{{width:12px;height:12px;border-radius:50%;display:inline-block}}
  .dot.up{{background:#00ff9d;box-shadow:0 0 8px #00ff9d80}}
  .dot.down{{background:#ff4444;box-shadow:0 0 8px #ff444480}}
  .dot.unknown{{background:#666}}
  .label{{font-size:.75rem;text-transform:uppercase;letter-spacing:.08em;color:#888;margin-bottom:4px}}
  .value{{font-size:1.5rem;font-family:"JetBrains Mono","Fira Code",monospace;color:#fff;
         margin-bottom:20px;word-break:break-all}}
  .value.accent{{color:#00ff9d}}
  .grid{{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:20px}}
  .grid .value{{font-size:1rem;margin-bottom:0}}
  .footer{{font-size:.7rem;color:#555;text-align:center;margin-top:24px;padding-top:16px;
          border-top:1px solid #222}}
  .error{{color:#ff4444;font-size:.9rem;padding:16px;background:#1a0a0a;border-radius:8px;
         border:1px solid #331111}}
  .refresh{{font-size:.75rem;color:#00ff9d;text-decoration:none;float:right;margin-top:-32px}}
  .refresh:hover{{text-decoration:underline}}
'''

HTML_TEMPLATE = '''<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Gluetun VPN Status</title>
<style>''' + CSS + '''</style>
</head>
<body>
<div class="card">
  <h1><span class="dot {status_class}"></span> Gluetun VPN</h1>
  <a href="/" class="refresh">Refresh</a>
  {content}
  <div class="footer">Gluetun VPN Client &middot; Control API on port 8000</div>
</div>
</body>
</html>'''

CONNECTED_BLOCK = '''
  <div class="label">Public IP</div>
  <div class="value accent">{public_ip}</div>
  <div class="grid">
    <div><div class="label">Country</div><div class="value">{country}</div></div>
    <div><div class="label">Region</div><div class="value">{region}</div></div>
    <div><div class="label">City</div><div class="value">{city}</div></div>
    <div><div class="label">Status</div><div class="value accent">Connected</div></div>
  </div>
  {port_section}
'''

PORT_SECTION = '''
  <div style="border-top:1px solid #222;padding-top:16px;margin-top:4px">
    <div class="label">Forwarded Port</div>
    <div class="value accent" style="font-size:1.3rem">{forwarded_port}</div>
  </div>
'''

ERROR_BLOCK = '''<div class="error">{message}</div>'''


def fetch_status():
    result = {"ok": False}
    try:
        req = urllib.request.urlopen(GLUETUN_API + "/v1/publicip/ip", timeout=5)
        data = json.loads(req.read())
        result = {
            "ok": True,
            "public_ip": data.get("public_ip", "Unknown"),
            "country": data.get("country", "Unknown"),
            "region": data.get("region", "Unknown"),
            "city": data.get("city", "Unknown"),
            "forwarded_port": None,
        }
    except Exception as e:
        return {"ok": False, "error": str(e)}
    # Try to get forwarded port (only present when VPN_PORT_FORWARDING=on)
    try:
        req = urllib.request.urlopen(GLUETUN_API + "/v1/portforward", timeout=3)
        pf = json.loads(req.read())
        port = pf.get("port", 0)
        if port and port > 0:
            result["forwarded_port"] = str(port)
    except Exception:
        pass
    return result


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        status = fetch_status()
        if status["ok"]:
            port_section = ""
            if status.get("forwarded_port"):
                port_section = PORT_SECTION.format(
                    forwarded_port=html.escape(status["forwarded_port"]),
                )
            content = CONNECTED_BLOCK.format(
                public_ip=html.escape(status["public_ip"]),
                country=html.escape(status["country"]),
                region=html.escape(status["region"]),
                city=html.escape(status["city"]),
                port_section=port_section,
            )
            status_class = "up"
        else:
            content = ERROR_BLOCK.format(message=html.escape(status["error"]))
            status_class = "down"

        page = HTML_TEMPLATE.format(content=content, status_class=status_class)
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(page.encode())

    def log_message(self, format, *args):
        pass  # suppress request logging


if __name__ == "__main__":
    bind_ip = get_lan_ip()
    server = http.server.HTTPServer((bind_ip, PORT), Handler)
    print(f"Status page listening on {bind_ip}:{PORT} (LAN only)")
    server.serve_forever()
"""


class GluetunApp(BaseApp):

    def _oci_request(self, url, token=None):
        """Make an HTTP request to the OCI registry, returning parsed JSON."""
        req = urllib.request.Request(url)
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        # Accept both OCI and Docker manifest types
        req.add_header("Accept", ", ".join([
            "application/vnd.oci.image.index.v1+json",
            "application/vnd.oci.image.manifest.v1+json",
            "application/vnd.docker.distribution.manifest.list.v2+json",
            "application/vnd.docker.distribution.manifest.v2+json",
        ]))
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read())

    def _oci_download(self, url, token=None):
        """Download raw bytes from the OCI registry."""
        req = urllib.request.Request(url)
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        with urllib.request.urlopen(req, timeout=120) as resp:
            return resp.read()

    def _pull_binary(self):
        """Download the Gluetun binary from Docker Hub OCI registry."""
        machine = platform.machine()
        if machine in ("x86_64", "AMD64"):
            arch = "amd64"
        elif machine in ("aarch64", "arm64"):
            arch = "arm64"
        else:
            raise RuntimeError(f"Unsupported architecture: {machine}")

        self.log.info(f"Pulling Gluetun binary for {arch} from Docker Hub...")

        # Step 1: Get auth token
        token_url = (
            f"https://auth.docker.io/token"
            f"?service=registry.docker.io"
            f"&scope=repository:{GLUETUN_IMAGE}:pull"
        )
        token_data = self._oci_request(token_url)
        token = token_data["token"]

        # Step 2: Fetch manifest list / index
        manifest_url = (
            f"https://registry-1.docker.io/v2/{GLUETUN_IMAGE}"
            f"/manifests/{GLUETUN_TAG}"
        )
        index = self._oci_request(manifest_url, token)

        # Find the manifest for our architecture
        arch_digest = None
        manifests = index.get("manifests", [])
        for m in manifests:
            p = m.get("platform", {})
            if p.get("architecture") == arch and p.get("os") == "linux":
                arch_digest = m["digest"]
                break

        if not arch_digest:
            raise RuntimeError(
                f"No linux/{arch} manifest found in {GLUETUN_IMAGE}:{GLUETUN_TAG}"
            )

        self.log.info(f"Found {arch} manifest: {arch_digest[:20]}...")

        # Step 3: Fetch the image manifest for our architecture
        img_manifest_url = (
            f"https://registry-1.docker.io/v2/{GLUETUN_IMAGE}"
            f"/manifests/{arch_digest}"
        )
        img_manifest = self._oci_request(img_manifest_url, token)

        # Step 4: Download layers and extract the gluetun binary
        layers = img_manifest.get("layers", [])
        if not layers:
            # Docker v2 schema uses "fsLayers" or nested config
            layers = img_manifest.get("layers", img_manifest.get("fsLayers", []))

        self.log.info(f"Scanning {len(layers)} layers for gluetun binary...")

        for i, layer in enumerate(layers):
            digest = layer["digest"]
            blob_url = (
                f"https://registry-1.docker.io/v2/{GLUETUN_IMAGE}"
                f"/blobs/{digest}"
            )
            self.log.info(f"Downloading layer {i+1}/{len(layers)} ({digest[:20]}...)...")
            blob = self._oci_download(blob_url, token)

            # Try to extract gluetun binary from this layer
            try:
                with tarfile.open(fileobj=io.BytesIO(blob), mode="r:*") as tar:
                    for member in tar.getmembers():
                        name = member.name.lstrip("./")
                        if name in ("gluetun", "gluetun-entrypoint") and member.isfile():
                            self.log.info(
                                f"Found gluetun binary in layer {i+1} "
                                f"({member.size // 1024 // 1024}MB)"
                            )
                            f = tar.extractfile(member)
                            data = f.read()
                            with open(BINARY_PATH, "wb") as out:
                                out.write(data)
                            self.run_command(["chmod", "755", BINARY_PATH])
                            self.log.info(f"Installed gluetun binary at {BINARY_PATH}")
                            return
            except (tarfile.TarError, EOFError):
                # Not a tar layer or doesn't contain what we need
                continue

        raise RuntimeError("gluetun binary not found in any image layer")

    # Environment variables that must not be overridden via extra_env.
    # These protect the kill switch, DNS leak prevention, and firewall integrity.
    BLOCKED_ENV_KEYS = frozenset({
        # DNS leak prevention — disabling these leaks DNS outside the VPN tunnel
        "DNS_SERVER",              # must stay "on" to use built-in encrypted DNS
        "DNS_KEEP_NAMESERVER",     # "on" leaks DNS through system resolver
        "DNS_ADDRESS",             # overriding routes DNS outside tunnel
        "DNS_UPSTREAM_RESOLVER_TYPE",  # must stay "dot" or "doh" (not "plain")
        # Kill switch bypass — these could route traffic outside the VPN
        "FIREWALL_OUTBOUND_SUBNETS",  # 0.0.0.0/0 would disable the kill switch
    })

    def _build_env(self):
        """Build the /etc/gluetun/env file from inputs."""
        lines = []

        # Security defaults (defense-in-depth) — Gluetun's own defaults,
        # set explicitly so they're visible and cannot be accidentally removed.
        lines.append("DNS_SERVER=on")
        lines.append("DNS_UPSTREAM_RESOLVER_TYPE=dot")
        lines.append("DNS_UPSTREAM_RESOLVERS=cloudflare")
        lines.append("DNS_KEEP_NAMESERVER=off")
        lines.append("BLOCK_MALICIOUS=on")
        lines.append("PPROF_ENABLED=no")
        lines.append("PPROF_BLOCK_PROFILE_RATE=0")
        lines.append("PPROF_MUTEX_PROFILE_RATE=0")
        provider = self.inputs.string("vpn_provider", "")
        vpn_type = self.inputs.string("vpn_type", "wireguard")
        lines.append(f"VPN_SERVICE_PROVIDER={provider}")
        lines.append(f"VPN_TYPE={vpn_type}")

        # Port forwarding
        port_fwd = self.inputs.string("vpn_port_forwarding", "false")
        if port_fwd.lower() in ("true", "on", "1", "yes"):
            lines.append("VPN_PORT_FORWARDING=on")

        # OpenVPN auth
        openvpn_user = self.inputs.string("openvpn_user", "")
        openvpn_pass = self.inputs.string("openvpn_password", "")
        if openvpn_user:
            lines.append(f"OPENVPN_USER={openvpn_user}")
        if openvpn_pass:
            lines.append(f"OPENVPN_PASSWORD={openvpn_pass}")

        # WireGuard auth
        wg_key = self.inputs.string("wireguard_private_key", "")
        wg_addrs = self.inputs.string("wireguard_addresses", "")
        wg_psk = self.inputs.string("wireguard_preshared_key", "")
        if wg_key:
            lines.append(f"WIREGUARD_PRIVATE_KEY={wg_key}")
        if wg_addrs:
            lines.append(f"WIREGUARD_ADDRESSES={wg_addrs}")
        if wg_psk:
            lines.append(f"WIREGUARD_PRESHARED_KEY={wg_psk}")
        wg_keepalive = self.inputs.string("wireguard_keepalive", "")
        if wg_keepalive:
            lines.append(f"WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL={wg_keepalive}")

        # Server selection
        for key, env in [
            ("server_countries", "SERVER_COUNTRIES"),
            ("server_regions", "SERVER_REGIONS"),
            ("server_cities", "SERVER_CITIES"),
            ("server_hostnames", "SERVER_HOSTNAMES"),
        ]:
            val = self.inputs.string(key, "")
            if val:
                lines.append(f"{env}={val}")

        # Proxy settings
        httpproxy = self.inputs.string("httpproxy", "true")
        httpproxy_port = self.inputs.string("httpproxy_port", "8888")
        if httpproxy.lower() in ("true", "on", "1", "yes"):
            lines.append("HTTPPROXY=on")
            lines.append(f"HTTPPROXY_LISTENING_ADDRESS=:{httpproxy_port}")
        else:
            lines.append("HTTPPROXY=off")

        shadowsocks = self.inputs.string("shadowsocks", "false")
        shadowsocks_port = self.inputs.string("shadowsocks_port", "8388")
        if shadowsocks.lower() in ("true", "on", "1", "yes"):
            lines.append("SHADOWSOCKS=on")
            lines.append(f"SHADOWSOCKS_LISTENING_ADDRESS=:{shadowsocks_port}")
        else:
            lines.append("SHADOWSOCKS=off")

        # Advanced
        tz = self.inputs.string("timezone", "")
        if tz:
            lines.append(f"TZ={tz}")

        updater_period = self.inputs.string("updater_period", "24h")
        if updater_period:
            lines.append(f"UPDATER_PERIOD={updater_period}")

        firewall_ports = self.inputs.string("firewall_vpn_input_ports", "")
        if firewall_ports:
            lines.append(f"FIREWALL_VPN_INPUT_PORTS={firewall_ports}")

        # Extra env: parse KEY=VALUE lines, blocking security-critical overrides
        extra = self.inputs.string("extra_env", "")
        if extra:
            for line in extra.strip().splitlines():
                line = line.strip()
                if line and "=" in line:
                    key = line.split("=", 1)[0].strip().upper()
                    if key in self.BLOCKED_ENV_KEYS:
                        self.log.warn(
                            f"Blocked extra_env override of {key} "
                            f"(security-critical setting)"
                        )
                        continue
                    lines.append(line)

        return "\n".join(lines) + "\n"

    def _verify_tun_device(self):
        """Verify /dev/net/tun exists (provided by LXC host config)."""
        if os.path.exists("/dev/net/tun"):
            self.log.info("/dev/net/tun is available")
        else:
            self.log.warn(
                "/dev/net/tun not found — the TUN device should be "
                "configured via LXC extra_config on the host"
            )

    def _wait_for_service(self, timeout=60):
        """Poll the Gluetun control API until it responds."""
        url = "http://127.0.0.1:8000/v1/publicip/ip"
        self.log.info("Waiting for Gluetun to establish VPN connection...")
        for i in range(timeout // 3):
            try:
                resp = urllib.request.urlopen(url, timeout=3)
                data = json.loads(resp.read())
                if data.get("public_ip"):
                    self.log.info(f"VPN connected — public IP: {data['public_ip']}")
                    return True
            except Exception:
                pass
            time.sleep(3)
        self.log.warn(
            "Gluetun did not report a VPN IP within timeout. "
            "Check logs with: journalctl -u gluetun"
        )
        return False

    def _disable_ipv6(self):
        """Disable IPv6 to prevent traffic leaking outside the VPN tunnel."""
        self.log.info("Disabling IPv6 (leak prevention)...")
        sysctl_conf = (
            "net.ipv6.conf.all.disable_ipv6 = 1\n"
            "net.ipv6.conf.default.disable_ipv6 = 1\n"
        )
        self.write_config("/etc/sysctl.d/99-disable-ipv6.conf", sysctl_conf)
        self.run_command(["sysctl", "--system"], check=False)

    def install(self):
        # Install system prerequisites
        self.apt_install(
            "openvpn", "wireguard-tools", "iptables",
            "ca-certificates", "kmod", "curl", "jq",
        )

        # Disable IPv6 to prevent leaks outside VPN tunnel
        self._disable_ipv6()

        # Verify TUN device (configured by engine via lxc.mount.entry)
        self._verify_tun_device()

        # Download Gluetun binary from Docker Hub OCI registry
        self._pull_binary()

        # Alpine compatibility — Gluetun is built for Alpine Linux
        self.log.info("Setting up Alpine compatibility layer...")
        self.write_config("/etc/alpine-release", "3.20.0\n")
        self.run_command(["ln", "-sf", "/usr/sbin/openvpn", "/usr/sbin/openvpn2.6"])

        # Create Gluetun data directory (servers.json, port forwarding, etc.)
        self.create_dir("/gluetun/")
        self.run_command(["chmod", "755", "/gluetun"])
        self.create_dir("/tmp/gluetun/")
        self.run_command(["chmod", "755", "/tmp/gluetun"])

        # Build environment config
        self.create_dir("/etc/gluetun/")
        env_content = self._build_env()
        self.write_config("/etc/gluetun/env", env_content)
        # Restrict permissions on env file (contains secrets)
        self.run_command(["chmod", "600", "/etc/gluetun/env"])

        # Install start script and systemd service
        self.write_config("/etc/gluetun/start.sh", START_SCRIPT)
        self.run_command(["chmod", "755", "/etc/gluetun/start.sh"])
        self.write_config("/etc/systemd/system/gluetun.service", SYSTEMD_UNIT)

        # Enable and start
        self.enable_service("gluetun")

        # Install VPN status page
        status_port = self.inputs.string("status_port", "8001")
        self.write_config("/etc/gluetun/status.py", STATUS_PAGE_PY,
                          status_port=status_port)
        self.write_config("/etc/systemd/system/gluetun-status.service",
                          STATUS_SYSTEMD_UNIT)
        self.enable_service("gluetun-status")

        # Wait for VPN to connect
        self._wait_for_service(timeout=60)

        self.log.info("Gluetun VPN client installed successfully")


run(GluetunApp)
