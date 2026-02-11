#!/usr/bin/env python3
"""
SWAG — Secure Web Application Gateway (LXC)

nginx reverse proxy with Let's Encrypt SSL, fail2ban,
and 300+ preset proxy configs for popular apps.

Based on linuxserver/docker-swag, adapted for LXC.
"""
from appstore import BaseApp, run


class Swag(BaseApp):

    def install(self):
        # ── Read inputs ─────────────────────────────────────────────
        url         = self.inputs.string("url", "")
        validation  = self.inputs.string("validation", "http")
        dnsplugin   = self.inputs.string("dnsplugin", "cloudflare")
        email       = self.inputs.string("email", "")
        subdomains  = self.inputs.string("subdomains", "wildcard")
        only_sub    = self.inputs.boolean("only_subdomains", False)
        staging     = self.inputs.boolean("staging", False)
        extra       = self.inputs.string("extra_domains", "")
        port_http   = self.inputs.integer("port_http", 80)
        port_https  = self.inputs.integer("port_https", 443)

        # ── Install system packages ─────────────────────────────────
        self.log.info("Installing system packages...")
        self.pkg_install(
            "bash", "ca-certificates", "coreutils", "curl", "jq",
            "openssl", "nginx", "nginx-mod-http-brotli",
            "nginx-mod-http-headers-more", "nginx-mod-stream",
            "fail2ban", "gnupg", "iptables-legacy", "logrotate",
            "python3", "py3-pip", "apache2-utils", "git",
            "inotify-tools",
        )

        # ── Install certbot + all DNS plugins in /lsiopy venv ───────
        # Matches the full linuxserver/docker-swag plugin set
        self.log.info("Installing certbot and DNS plugins...")
        self.pip_install(
            "certbot",
            "certbot-dns-acmedns",
            "certbot-dns-aliyun",
            "certbot-dns-azure",
            "certbot-dns-bunny",
            "certbot-dns-cloudflare",
            "certbot-dns-cpanel",
            "certbot-dns-desec",
            "certbot-dns-digitalocean",
            "certbot-dns-directadmin",
            "certbot-dns-dnsimple",
            "certbot-dns-dnsmadeeasy",
            "certbot-dns-dnspod",
            "certbot-dns-do",
            "certbot-dns-domeneshop",
            "certbot-dns-dreamhost",
            "certbot-dns-duckdns",
            "certbot-dns-dynudns",
            "certbot-dns-freedns",
            "certbot-dns-gehirn",
            "certbot-dns-glesys",
            "certbot-dns-godaddy",
            "certbot-dns-google",
            "certbot-dns-he",
            "certbot-dns-hetzner",
            "certbot-dns-infomaniak",
            "certbot-dns-inwx",
            "certbot-dns-ionos",
            "certbot-dns-linode",
            "certbot-dns-loopia",
            "certbot-dns-luadns",
            "certbot-dns-namecheap",
            "certbot-dns-netcup",
            "certbot-dns-njalla",
            "certbot-dns-nsone",
            "certbot-dns-ovh",
            "certbot-dns-porkbun",
            "certbot-dns-rfc2136",
            "certbot-dns-route53",
            "certbot-dns-sakuracloud",
            "certbot-dns-standalone",
            "certbot-dns-transip",
            "certbot-dns-vultr",
            "certbot-plugin-gandi",
            "cryptography",
            "requests",
            venv="/lsiopy",
        )

        # ── Create directory structure ──────────────────────────────
        self.log.info("Creating config directory structure...")
        for d in [
            "/config/nginx/site-confs",
            "/config/nginx/proxy-confs",
            "/config/dns-conf",
            "/config/keys",
            "/config/www",
            "/config/log/nginx",
            "/config/log/letsencrypt",
            "/config/log/fail2ban",
            "/config/fail2ban",
            "/config/etc/letsencrypt/renewal-hooks/deploy",
            "/tmp/letsencrypt",
            "/run/nginx",
            "/run/fail2ban",
        ]:
            self.create_dir(d)

        # ── Deploy nginx configs from templates ─────────────────────
        self.log.info("Deploying nginx configuration...")
        self.deploy_provision_file("nginx.conf", "/etc/nginx/nginx.conf")
        self.deploy_provision_file("ssl.conf", "/config/nginx/ssl.conf")
        self.deploy_provision_file("proxy.conf", "/config/nginx/proxy.conf")
        self.deploy_provision_file("default-site.conf",
                                   "/config/nginx/site-confs/default.conf")
        self.deploy_provision_file("index.html", "/config/www/index.html")

        # Remove Alpine default site (we use our own)
        self.run_command(["rm", "-f", "/etc/nginx/http.d/default.conf"],
                         check=False)

        # ── Download preset reverse proxy configs ───────────────────
        self.log.info("Downloading 300+ preset proxy configs...")
        self.download(
            "https://github.com/linuxserver/reverse-proxy-confs/tarball/master",
            "/tmp/proxy-confs.tar.gz",
        )
        self.run_command([
            "tar", "xf", "/tmp/proxy-confs.tar.gz",
            "-C", "/config/nginx/proxy-confs",
            "--strip-components=1",
            "--exclude=linux*/.editorconfig",
            "--exclude=linux*/.gitattributes",
            "--exclude=linux*/.github",
            "--exclude=linux*/.gitignore",
            "--exclude=linux*/LICENSE",
        ], check=False)
        self.run_command(["rm", "-f", "/tmp/proxy-confs.tar.gz"])

        # ── Clone SWAG defaults (dns-conf templates, fail2ban) ──────
        self.log.info("Fetching DNS credential templates and fail2ban configs...")
        self.run_command([
            "git", "clone", "--depth", "1",
            "https://github.com/linuxserver/docker-swag.git",
            "/tmp/_swag",
        ])

        # Copy DNS credential templates
        self.run_command([
            "cp", "-rn", "/tmp/_swag/root/defaults/dns-conf/.",
            "/config/dns-conf/",
        ], check=False)

        # Copy fail2ban filter and action definitions
        self.run_command([
            "cp", "-r", "/tmp/_swag/root/defaults/fail2ban/filter.d",
            "/config/fail2ban/",
        ], check=False)
        self.run_command([
            "cp", "-r", "/tmp/_swag/root/defaults/fail2ban/action.d",
            "/config/fail2ban/",
        ], check=False)

        self.run_command(["rm", "-rf", "/tmp/_swag"])

        # ── Deploy fail2ban config ──────────────────────────────────
        self.log.info("Configuring fail2ban...")
        self.deploy_provision_file("jail.local",
                                   "/config/fail2ban/jail.local")

        # Symlink user configs into fail2ban expected paths
        self.run_command(["rm", "-rf", "/etc/fail2ban/filter.d"])
        self.run_command(["rm", "-rf", "/etc/fail2ban/action.d"])
        self.run_command([
            "ln", "-sf", "/config/fail2ban/filter.d", "/etc/fail2ban/filter.d",
        ])
        self.run_command([
            "ln", "-sf", "/config/fail2ban/action.d", "/etc/fail2ban/action.d",
        ])
        self.run_command([
            "cp", "/config/fail2ban/jail.local", "/etc/fail2ban/jail.local",
        ])

        # Create empty log files (fail2ban needs them to exist)
        for log in ["/config/log/nginx/error.log",
                    "/config/log/nginx/access.log"]:
            self.run_command(["touch", log])

        # Fix iptables symlinks for Alpine
        self.run_command([
            "ln", "-sf", "/usr/sbin/xtables-legacy-multi",
            "/usr/sbin/iptables",
        ], check=False)
        self.run_command([
            "ln", "-sf", "/usr/sbin/xtables-legacy-multi",
            "/usr/sbin/iptables-save",
        ], check=False)
        self.run_command([
            "ln", "-sf", "/usr/sbin/xtables-legacy-multi",
            "/usr/sbin/iptables-restore",
        ], check=False)

        # ── Generate self-signed cert (so nginx starts immediately) ─
        self.log.info("Generating self-signed certificate...")
        self.run_command([
            "openssl", "req", "-x509", "-nodes",
            "-days", "365",
            "-newkey", "rsa:2048",
            "-keyout", "/config/keys/cert.key",
            "-out", "/config/keys/cert.crt",
            "-subj", "/CN=swag-selfsigned",
        ])

        # ── Request Let's Encrypt cert (if domain is set) ──────────
        if url:
            self.log.info(f"Requesting certificate for {url}...")
            self._request_certificate(
                url, validation, dnsplugin, email,
                subdomains, only_sub, staging, extra,
            )

        # ── Set up auto-renewal cron job ────────────────────────────
        self.deploy_provision_file(
            "certbot-renew.sh", "/etc/periodic/daily/certbot-renew",
            mode="0755",
        )

        # ── Enable and start services ───────────────────────────────
        self.log.info("Starting services...")
        self.enable_service("nginx")
        self.enable_service("fail2ban")
        self.restart_service("nginx")
        self.restart_service("fail2ban")

        self.log.info("SWAG installation complete")

    def configure(self):
        """Reconfigure — re-request certificate with updated inputs."""
        url         = self.inputs.string("url", "")
        validation  = self.inputs.string("validation", "http")
        dnsplugin   = self.inputs.string("dnsplugin", "cloudflare")
        email       = self.inputs.string("email", "")
        subdomains  = self.inputs.string("subdomains", "wildcard")
        only_sub    = self.inputs.boolean("only_subdomains", False)
        staging     = self.inputs.boolean("staging", False)
        extra       = self.inputs.string("extra_domains", "")

        if url:
            self.log.info(f"Re-requesting certificate for {url}...")
            self._request_certificate(
                url, validation, dnsplugin, email,
                subdomains, only_sub, staging, extra,
            )
            self.restart_service("nginx")

    def _request_certificate(self, url, validation, dnsplugin, email,
                              subdomains, only_sub, staging, extra):
        """Build certbot command and request a certificate."""
        certbot = "/lsiopy/bin/certbot"

        # Build domain list
        domains = []
        if not only_sub:
            domains.append(url)
        if subdomains:
            if subdomains == "wildcard":
                domains.append(f"*.{url}")
            else:
                for sub in subdomains.split(","):
                    sub = sub.strip()
                    if sub:
                        domains.append(f"{sub}.{url}")
        if extra:
            for d in extra.split(","):
                d = d.strip()
                if d:
                    domains.append(d)

        if not domains:
            self.log.warning("No domains to request certificate for")
            return

        # Base certbot command
        cmd = [
            certbot, "certonly",
            "--config-dir", "/config/etc/letsencrypt",
            "--logs-dir", "/config/log/letsencrypt",
            "--work-dir", "/tmp/letsencrypt",
            "--non-interactive",
            "--agree-tos",
            "--renew-by-default",
        ]

        # Add domains
        for d in domains:
            cmd.extend(["-d", d])

        # Email or register without
        if email and "@" in email:
            cmd.extend(["--email", email, "--no-eff-email"])
        else:
            cmd.append("--register-unsafely-without-email")

        # Staging server
        if staging:
            cmd.extend([
                "--server",
                "https://acme-staging-v02.api.letsencrypt.org/directory",
            ])

        # Validation method
        if validation == "dns":
            plugin = f"dns-{dnsplugin}"
            cmd.extend([
                "--authenticator", plugin,
                "--preferred-challenges", "dns",
            ])
            # Credential file (most plugins need one)
            if dnsplugin not in ("route53", "standalone"):
                ext = "json" if dnsplugin == "google" else "ini"
                cred_file = f"/config/dns-conf/{dnsplugin}.{ext}"
                cmd.extend([f"--{plugin}-credentials", cred_file])
        else:
            cmd.extend([
                "--authenticator", "standalone",
                "--preferred-challenges", "http",
            ])

        self.log.info(f"Certbot: requesting cert for {', '.join(domains)}")
        self.run_command(cmd, check=False)

        # Update cert symlinks if certbot succeeded
        if only_sub and subdomains != "wildcard":
            first_sub = subdomains.split(",")[0].strip()
            cert_domain = f"{first_sub}.{url}"
        else:
            cert_domain = url

        cert_path = f"/config/etc/letsencrypt/live/{cert_domain}"

        # If cert was generated, replace self-signed with symlinks
        self.run_command(["sh", "-c", f"""
            if [ -d {cert_path} ]; then
                rm -f /config/keys/cert.crt /config/keys/cert.key
                ln -s {cert_path}/fullchain.pem /config/keys/cert.crt
                ln -s {cert_path}/privkey.pem /config/keys/cert.key
                echo "Certificate installed successfully"
            else
                echo "Certificate not found at {cert_path} — using self-signed cert"
            fi
        """])


run(Swag)
