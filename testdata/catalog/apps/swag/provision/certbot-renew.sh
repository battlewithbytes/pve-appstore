#!/bin/sh
## SWAG LXC — Certbot auto-renewal script (runs via cron)
## Renews certificates and reloads nginx on success

/lsiopy/bin/certbot renew \
    --config-dir /config/etc/letsencrypt \
    --logs-dir /config/log/letsencrypt \
    --work-dir /tmp/letsencrypt \
    --non-interactive \
    --deploy-hook "rc-service nginx reload" \
    2>&1 | logger -t certbot-renew
