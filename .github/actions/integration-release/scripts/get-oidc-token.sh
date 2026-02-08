#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" || -z "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]]; then
  echo "OIDC token request env not available" >&2
  exit 1
fi

if [[ -z "${OIDC_AUDIENCE:-}" ]]; then
  echo "OIDC_AUDIENCE is required" >&2
  exit 1
fi

OIDC_URL="${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=${OIDC_AUDIENCE}"
OIDC_JSON=$(curl -fsS -H "Authorization: Bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" "$OIDC_URL")
OIDC_TOKEN=$(echo "$OIDC_JSON" | jq -r '.value // ""')

if [[ -z "$OIDC_TOKEN" || "$OIDC_TOKEN" == "null" ]]; then
  echo "Failed to obtain OIDC token" >&2
  exit 1
fi

echo "$OIDC_TOKEN"
