#!/usr/bin/env bash
set -euo pipefail

output_dir=${1:-artifacts/licenses/go}
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)

while IFS= read -r module; do
  module_dir=$(dirname "$module")
  module_name=$(printf '%s' "$module_dir" | sed 's#^\./##; s#[/ ]#-#g')
  (
    cd "$module_dir"
    go-licenses csv ./... > "$output_dir/${module_name:-root}.csv"
  )
done < <(find . -type f -name go.mod -not -path './vendor/*' -not -path './node_modules/*' 2>/dev/null | sort)