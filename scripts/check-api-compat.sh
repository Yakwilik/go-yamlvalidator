#!/usr/bin/env bash
set -euo pipefail

baseline="${1:-v1.0.0}"
module_path="$(go list -m -f '{{.Path}}')"
tool_version='v0.0.0-20260908205506-85c1c2202aba'

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

old_dir="$(
  go mod download -json "${module_path}@${baseline}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["Dir"])'
)"

GOBIN="$tmp_dir/bin" go install "golang.org/x/exp/cmd/apidiff@${tool_version}"
apidiff="$tmp_dir/bin/apidiff"

(
  cd "$old_dir"
  "$apidiff" -m -w "$tmp_dir/old.api" "$module_path"
)
"$apidiff" -m -w "$tmp_dir/new.api" "$module_path"
# apidiff reports incompatible changes on stdout but can still exit 0.
# Do not accidentally treat a printed incompatibility report as a passing gate.
"$apidiff" -m -incompatible "$tmp_dir/old.api" "$tmp_dir/new.api" > "$tmp_dir/incompatible.txt"
cat "$tmp_dir/incompatible.txt"
if [[ -s "$tmp_dir/incompatible.txt" ]]; then
  echo "API compatibility differences require review" >&2
  exit 1
fi
