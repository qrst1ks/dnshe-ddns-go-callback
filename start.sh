#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

KEY_FILE="${ROOT_DIR}/.config_master_key"

if [[ ! -f "$KEY_FILE" ]]; then
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 >"$KEY_FILE"
  else
    # Fallback: use /dev/urandom and base64 when openssl is unavailable
    head -c 32 /dev/urandom | base64 >"$KEY_FILE"
  fi
fi
chmod 600 "$KEY_FILE"

export CONFIG_MASTER_KEY="$(tr -d '\r\n' < "$KEY_FILE")"

if [[ -z "${PORT:-}" ]]; then
  export PORT=18491
fi

echo "Starting dnshe-ddns-go-callback on :${PORT}"
echo "Open: http://127.0.0.1:${PORT}/"

if [[ -x "./dnshe-ddns-go-callback" ]]; then
  ./dnshe-ddns-go-callback
else
  go run .
fi
