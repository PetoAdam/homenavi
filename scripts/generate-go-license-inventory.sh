#!/usr/bin/env bash
set -euo pipefail

output_dir=${1:-artifacts/licenses/go}
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)

scan_module() {
  local module_dir=$1
  local report=$2
  local attempt

  for attempt in 1 2 3; do
    if (
      cd "$module_dir"
      GODEBUG="${GODEBUG:+${GODEBUG},}http2client=0" go mod download
      GODEBUG="${GODEBUG:+${GODEBUG},}http2client=0" go-licenses csv ./... > "$report"
    ); then
      return 0
    fi

    rm -f "$report"
    echo "License inventory scan failed for $module_dir (attempt $attempt of 3)." >&2
  done

  echo "Unable to generate a license inventory for $module_dir." >&2
  return 1
}

while IFS= read -r module; do
  module_dir=$(dirname "$module")
  module_name=$(printf '%s' "$module_dir" | sed 's#^\./##; s#[/ ]#-#g')
  scan_module "$module_dir" "$output_dir/${module_name:-root}.csv"
done < <(find . -type f -name go.mod -not -path './vendor/*' -not -path './node_modules/*' 2>/dev/null | sort)