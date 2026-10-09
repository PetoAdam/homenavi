#!/usr/bin/env sh
set -eu

chart_dir=${1:-helm/homenavi}
rendered=$(mktemp)
trap 'rm -f "$rendered"' EXIT

helm template homenavi "$chart_dir" --set ingress.enabled=true >"$rendered"

assert_route() {
  expected_path=$1
  expected_type=$2
  expected_service=$3
  expected_port=$4

  if ! awk -v expected_path="$expected_path" -v expected_type="$expected_type" -v expected_service="$expected_service" -v expected_port="$expected_port" '
    $1 == "-" && $2 == "path:" { active = ($3 == expected_path); next }
    active && $1 == "pathType:" { path_type = $2; next }
    active && $1 == "name:" { service = $2; next }
    active && $1 == "number:" {
      if (path_type == expected_type && service == expected_service && $2 == expected_port) {
        found = 1
      }
      active = 0
    }
    END { exit(found ? 0 : 1) }
  ' "$rendered"; then
    echo "missing route: $expected_path -> $expected_service:$expected_port ($expected_type)" >&2
    exit 1
  fi
}

assert_route / Prefix edge-proxy 80

for upstream in http://api-gateway:8080 http://integration-proxy:8099 http://frontend:80; do
  if ! grep -Fq "proxy_pass $upstream;" "$rendered"; then
    echo "missing edge route upstream: $upstream" >&2
    exit 1
  fi
done

if ! grep -Fq 'return 301 /integrations/;' "$rendered"; then
  echo "missing /integrations canonical-path redirect" >&2
  exit 1
fi

if [ "$(grep -Fc 'proxy_set_header X-Forwarded-Host $host;' "$rendered")" -ne 6 ]; then
  echo "every edge upstream route must forward the public host" >&2
  exit 1
fi

for direct_path in /api /ws /mcp /.well-known /integrations; do
  if grep -Fq "path: $direct_path" "$rendered"; then
    echo "ingress must not route $direct_path directly" >&2
    exit 1
  fi
done