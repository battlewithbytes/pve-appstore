# SWAG — Secure Web Application Gateway

nginx reverse proxy with automated Let's Encrypt SSL certificates,
fail2ban intrusion prevention, and 300+ preset proxy configs for popular
self-hosted apps.

## Features

- **Automatic SSL** — Let's Encrypt via HTTP or DNS validation
- **Reverse Proxy** — 300+ preset configs for apps like Plex, Nextcloud, Home Assistant, etc.
- **fail2ban** — Blocks brute-force attacks on nginx (HTTP auth, bad bots, unauthorized access)
- **Self-signed fallback** — nginx starts immediately with a self-signed cert; Let's Encrypt replaces it when ready
- **Auto-renewal** — Daily cron job renews certs and reloads nginx

## Quick Start

1. Install the app with your domain and validation method
2. For DNS validation: edit `/config/dns-conf/<plugin>.ini` with your API credentials
3. Enable proxy configs by renaming `.conf.sample` to `.conf` in `/config/nginx/proxy-confs/`
4. Update the `$upstream_app` variable in each proxy conf to point to your service IP

## Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `url` | Your domain name | *(required)* |
| `validation` | `http` or `dns` | `http` |
| `dnsplugin` | DNS provider plugin | `cloudflare` |
| `email` | Let's Encrypt notification email | *(optional)* |
| `subdomains` | Comma-separated or `wildcard` | `wildcard` |
| `staging` | Use LE staging server | `false` |

## Directory Structure

```
/config/
  dns-conf/          # DNS credential files (cloudflare.ini, etc.)
  etc/letsencrypt/   # Certbot config and certificates
  fail2ban/          # fail2ban jails, filters, actions
  keys/              # cert.crt and cert.key (symlinks to LE certs)
  log/               # nginx, letsencrypt, fail2ban logs
  nginx/
    proxy-confs/     # 300+ reverse proxy configs (.conf.sample)
    site-confs/      # Site configs (default.conf)
    proxy.conf       # Proxy header settings
    ssl.conf         # SSL/TLS settings
  www/               # Web root (default landing page)
```

## Based On

[linuxserver/docker-swag](https://github.com/linuxserver/docker-swag) — adapted for LXC containers.
