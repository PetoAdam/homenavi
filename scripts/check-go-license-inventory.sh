#!/usr/bin/env bash
set -euo pipefail

inventory_dir=${1:-artifacts/licenses/go}

if [[ ! -d "$inventory_dir" ]]; then
  echo "Go license inventory directory not found: $inventory_dir" >&2
  exit 1
fi

if ! unresolved=$(awk -F, '
  $1 !~ /^github.com\/PetoAdam\/homenavi(\/|$)/ && tolower($2) ~ /^(unknown|none)$/ {
    print FILENAME ":" $0
    found = 1
  }
  END { exit found }
' "$inventory_dir"/*.csv); then
  printf '%s\n' "$unresolved" >&2
  echo 'Go dependency inventory contains unresolved external license metadata.' >&2
  exit 1
fi
