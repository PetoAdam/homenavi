#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

echo "==> Go tests"
go test ./...

echo "==> Manifest schema validation"
go run ./cmd/validate-manifest

echo "==> Frontend build"
cd "$ROOT_DIR/src/frontend"
if [[ -f package-lock.json ]]; then
	npm ci
else
	npm install
fi
npm run build

cd "$ROOT_DIR"

echo "==> Docker build"
docker build -t homenavi-integration:local .

echo "All checks passed."
