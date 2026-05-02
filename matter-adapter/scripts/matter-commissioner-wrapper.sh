#!/bin/sh

set -eu

base_url="${MATTER_COMMISSIONER_URL:-http://matter-commissioner:8098}"
command_name="${1:-}"

case "$command_name" in
  pair)
    endpoint="/pair"
    ;;
  command)
    endpoint="/command"
    ;;
  *)
    echo "unsupported commissioner command: $command_name" >&2
    exit 64
    ;;
esac

payload_file="$(mktemp)"
response_file="$(mktemp)"
trap 'rm -f "$payload_file" "$response_file"' EXIT

cat >"$payload_file"

status_code="$({
  curl -sS \
    -o "$response_file" \
    -w '%{http_code}' \
    -X POST \
    -H "Content-Type: application/json" \
    --data-binary @"$payload_file" \
    "${base_url%/}${endpoint}"
} )"

case "$status_code" in
  2??)
    cat "$response_file"
    ;;
  *)
    if [ -s "$response_file" ]; then
      cat "$response_file" >&2
    else
      echo "commissioner request failed with status $status_code" >&2
    fi
    exit 22
    ;;
esac