#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

echo "==> Running Go tests for shared packages and migrated services"
go test ./shared/... ./history-service/... ./automation-service/... ./entity-registry-service/... ./device-hub/... ./integration-proxy/... ./auth-service/... ./email-service/... ./dashboard-service/... ./api-gateway/... ./user-service/... ./weather-service/... ./thread-adapter/... ./zigbee-adapter/...

echo "==> Validating docker-compose configuration"
docker compose -f docker-compose.yml config >/dev/null

echo "==> Validating Helm chart"
helm lint ./helm/homenavi
helm template homenavi ./helm/homenavi >/tmp/homenavi-rendered.yaml

echo "Validation passed"