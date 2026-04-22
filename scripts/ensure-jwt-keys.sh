#!/usr/bin/env bash
set -euo pipefail

OUT_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      OUT_DIR="${2:-}"
      if [[ -z "$OUT_DIR" ]]; then
        echo "--dir requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: $0 [--dir <output-dir>]" >&2
      exit 1
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="$ROOT_DIR/keys"
fi

mkdir -p "$OUT_DIR"

private_key="$OUT_DIR/jwt_private.pem"
public_key="$OUT_DIR/jwt_public.pem"

if [[ ! -f "$private_key" || ! -f "$public_key" ]]; then
  rm -f "$private_key" "$public_key"
  openssl genrsa -out "$private_key" 2048 >/dev/null 2>&1
  openssl rsa -in "$private_key" -pubout -out "$public_key" >/dev/null 2>&1
  echo "Generated JWT keypair in $OUT_DIR"
else
  echo "JWT keypair already present in $OUT_DIR"
fi