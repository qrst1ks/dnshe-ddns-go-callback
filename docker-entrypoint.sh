#!/bin/sh
set -eu

DATA_DIR="${DATA_DIR:-/data}"
KEY_FILE="${KEY_FILE:-$DATA_DIR/.config_master_key}"

mkdir -p "$DATA_DIR"

if [ ! -f "$KEY_FILE" ]; then
  head -c 32 /dev/urandom | base64 > "$KEY_FILE"
fi
chmod 600 "$KEY_FILE"

export CONFIG_MASTER_KEY="$(tr -d '\r\n' < "$KEY_FILE")"
export CONFIG_PATH="${CONFIG_PATH:-$DATA_DIR/runtime_config.enc}"
export PORT="${PORT:-18491}"

exec /app/dnshe-ddns-go-callback
