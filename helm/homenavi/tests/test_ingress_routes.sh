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

assert_route / Prefix frontend 3000
assert_route /api Prefix api-gateway 8080
assert_route /ws Prefix api-gateway 8080
assert_route /mcp Exact api-gateway 8080
assert_route /.well-known Prefix api-gateway 8080
assert_route /integrations Prefix integration-proxy 8099