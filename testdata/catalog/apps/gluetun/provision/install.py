"""Gluetun VPN Client — multi-provider VPN with proxy servers and kill switch.

Downloads the Gluetun binary directly from Docker Hub's OCI registry
(no Docker required) and runs it as a systemd service in a privileged
LXC container.
"""

import os

from appstore import BaseApp, run

START_SCRIPT = """\
#!/bin/bash
set -a
source /etc/gluetun/env
exec /gluetun-entrypoint
"""

# Environment variables that must not be overridden via extra_env.
# These protect the kill switch, DNS leak prevention, and firewall integrity.
BLOCKED_ENV_KEYS = frozenset({
    "DNS_SERVER", "DNS_KEEP_NAMESERVER", "DNS_ADDRESS",
    "DNS_UPSTREAM_RESOLVER_TYPE", "FIREWALL_OUTBOUND_SUBNETS",
})


class GluetunApp(BaseApp):

    def _build_env(self):
        """Build the environment dict from inputs."""
        env = {}

        # Security defaults (defense-in-depth)
        env["DNS_SERVER"] = "on"
        env["DNS_UPSTREAM_RESOLVER_TYPE"] = "dot"
        env["DNS_UPSTREAM_RESOLVERS"] = "cloudflare"
        env["DNS_KEEP_NAMESERVER"] = "off"
        env["BLOCK_MALICIOUS"] = "on"
        env["PPROF_ENABLED"] = "no"
        env["PPROF_BLOCK_PROFILE_RATE"] = "0"
        env["PPROF_MUTEX_PROFILE_RATE"] = "0"

        # Provider config
        env["VPN_SERVICE_PROVIDER"] = self.inputs.string("vpn_provider", "")
        env["VPN_TYPE"] = self.inputs.string("vpn_type", "wireguard")

        # Port forwarding
        port_fwd = self.inputs.string("vpn_port_forwarding", "false")
        if port_fwd.lower() in ("true", "on", "1", "yes"):
            env["VPN_PORT_FORWARDING"] = "on"

        # OpenVPN auth
        env["OPENVPN_USER"] = self.inputs.string("openvpn_user", "")
        env["OPENVPN_PASSWORD"] = self.inputs.string("openvpn_password", "")

        # WireGuard auth
        env["WIREGUARD_PRIVATE_KEY"] = self.inputs.string("wireguard_private_key", "")
        env["WIREGUARD_ADDRESSES"] = self.inputs.string("wireguard_addresses", "")
        env["WIREGUARD_PRESHARED_KEY"] = self.inputs.string("wireguard_preshared_key", "")
        wg_keepalive = self.inputs.string("wireguard_keepalive", "")
        if wg_keepalive:
            env["WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL"] = wg_keepalive

        # Server selection
        for key, evar in [
            ("server_countries", "SERVER_COUNTRIES"),
            ("server_regions", "SERVER_REGIONS"),
            ("server_cities", "SERVER_CITIES"),
            ("server_hostnames", "SERVER_HOSTNAMES"),
        ]:
            val = self.inputs.string(key, "")
            if val:
                env[evar] = val

        # Proxy settings
        httpproxy = self.inputs.string("httpproxy", "true")
        httpproxy_port = self.inputs.string("httpproxy_port", "8888")
        if httpproxy.lower() in ("true", "on", "1", "yes"):
            env["HTTPPROXY"] = "on"
            env["HTTPPROXY_LISTENING_ADDRESS"] = f":{httpproxy_port}"
        else:
            env["HTTPPROXY"] = "off"

        shadowsocks = self.inputs.string("shadowsocks", "false")
        shadowsocks_port = self.inputs.string("shadowsocks_port", "8388")
        if shadowsocks.lower() in ("true", "on", "1", "yes"):
            env["SHADOWSOCKS"] = "on"
            env["SHADOWSOCKS_LISTENING_ADDRESS"] = f":{shadowsocks_port}"
        else:
            env["SHADOWSOCKS"] = "off"

        # Advanced
        tz = self.inputs.string("timezone", "")
        if tz:
            env["TZ"] = tz
        updater_period = self.inputs.string("updater_period", "24h")
        if updater_period:
            env["UPDATER_PERIOD"] = updater_period
        firewall_ports = self.inputs.string("firewall_vpn_input_ports", "")
        if firewall_ports:
            env["FIREWALL_VPN_INPUT_PORTS"] = firewall_ports

        # Extra env: parse KEY=VALUE lines, blocking security-critical overrides
        extra = self.inputs.string("extra_env", "")
        if extra:
            for line in extra.strip().splitlines():
                line = line.strip()
                if line and "=" in line:
                    k = line.split("=", 1)[0].strip().upper()
                    if k in BLOCKED_ENV_KEYS:
                        self.log.warn(
                            f"Blocked extra_env override of {k} "
                            f"(security-critical setting)"
                        )
                        continue
                    env[k] = line.split("=", 1)[1]

        return env

    def install(self):
        # System prerequisites
        self.apt_install(
            "openvpn", "wireguard-tools", "iptables",
            "ca-certificates", "kmod", "curl", "jq",
        )

        # Disable IPv6 to prevent leaks outside VPN tunnel
        self.disable_ipv6()

        # Verify TUN device
        if os.path.exists("/dev/net/tun"):
            self.log.info("/dev/net/tun is available")
        else:
            self.log.warn("/dev/net/tun not found — configure via LXC extra_config")

        # Download Gluetun binary from Docker Hub OCI registry
        self.pull_oci_binary("qmcgaw/gluetun", dest="/gluetun-entrypoint")

        # Alpine compatibility — Gluetun is built for Alpine Linux
        self.log.info("Setting up Alpine compatibility layer...")
        self.write_config("/etc/alpine-release", "3.20.0\n")
        self.run_command(["ln", "-sf", "/usr/sbin/openvpn", "/usr/sbin/openvpn2.6"])

        # Create data directories
        self.create_dir("/gluetun/")
        self.create_dir("/tmp/gluetun/")
        self.create_dir("/etc/gluetun/")

        # Build and write environment config
        self.write_env_file("/etc/gluetun/env", self._build_env(), mode="0600")

        # Install start script
        self.write_config("/etc/gluetun/start.sh", START_SCRIPT)
        self.run_command(["chmod", "755", "/etc/gluetun/start.sh"])

        # Create and start Gluetun service
        self.create_service("gluetun",
            exec_start="/etc/gluetun/start.sh",
            description="Gluetun VPN Client",
            capabilities=["CAP_NET_ADMIN", "CAP_NET_RAW", "CAP_NET_BIND_SERVICE"],
        )

        # Deploy VPN status page
        self.status_page(
            port=self.inputs.integer("status_port", 8001),
            title="Gluetun",
            api_url="http://127.0.0.1:8000/v1/publicip/ip",
            fields={
                "public_ip": "Public IP",
                "country": "Country",
                "region": "Region",
                "city": "City",
            },
        )

        # Wait for VPN to connect
        self.wait_for_http("http://127.0.0.1:8000/v1/publicip/ip", timeout=60)

        self.log.info("Gluetun VPN client installed successfully")


run(GluetunApp)
