#!/usr/bin/env bash
# qBittorrent Port Updater — polls Gluetun for forwarded port and updates qBittorrent
set -euo pipefail

: "${QBT_HOST:=127.0.0.1}"
: "${QBT_PORT:=8080}"
: "${QBT_USER:=admin}"
: "${QBT_PASSWORD:=}"
: "${GLUETUN_HOST:=127.0.0.1}"
: "${GLUETUN_PORT:=8000}"
: "${POLL_INTERVAL:=60}"

QBT_URL="http://${QBT_HOST}:${QBT_PORT}"
GLUETUN_URL="http://${GLUETUN_HOST}:${GLUETUN_PORT}"
CURRENT_PORT=""
COOKIE_JAR=$(mktemp)

cleanup() {
    rm -f "$COOKIE_JAR"
}
trap cleanup EXIT

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

qbt_login() {
    local response
    response=$(curl -fsSL -c "$COOKIE_JAR" -d "username=${QBT_USER}&password=${QBT_PASSWORD}" \
        "${QBT_URL}/api/v2/auth/login" 2>&1) || true
    if [ "$response" = "Ok." ]; then
        return 0
    fi
    log "WARN: qBittorrent login failed: ${response}"
    return 1
}

get_forwarded_port() {
    local response port
    response=$(curl -fsSL "${GLUETUN_URL}/v1/openvpn/portforwarded" 2>/dev/null) || {
        # Try the newer endpoint
        response=$(curl -fsSL "${GLUETUN_URL}/v1/portforward" 2>/dev/null) || return 1
    }
    port=$(echo "$response" | jq -r '.port // empty' 2>/dev/null)
    if [ -z "$port" ] || [ "$port" = "0" ]; then
        return 1
    fi
    echo "$port"
}

set_qbt_port() {
    local port=$1
    curl -fsSL -b "$COOKIE_JAR" -d "json={\"listen_port\":${port}}" \
        "${QBT_URL}/api/v2/app/setPreferences" >/dev/null 2>&1
}

log "Starting qBittorrent Port Updater"
log "Gluetun: ${GLUETUN_URL} | qBittorrent: ${QBT_URL}"
log "Poll interval: ${POLL_INTERVAL}s"

while true; do
    forwarded_port=$(get_forwarded_port) || {
        log "Waiting for Gluetun forwarded port..."
        sleep "$POLL_INTERVAL"
        continue
    }

    if [ "$forwarded_port" != "$CURRENT_PORT" ]; then
        log "Port change detected: ${CURRENT_PORT:-none} -> ${forwarded_port}"

        if qbt_login; then
            if set_qbt_port "$forwarded_port"; then
                CURRENT_PORT="$forwarded_port"
                log "Updated qBittorrent listen port to ${forwarded_port}"
            else
                log "ERROR: Failed to update qBittorrent port"
            fi
        fi
    fi

    sleep "$POLL_INTERVAL"
done
