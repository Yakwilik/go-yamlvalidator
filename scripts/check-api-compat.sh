#!/usr/bin/env bash
set -euo pipefail

baseline="${1:-v0.6.0}"
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
"$apidiff" -m -incompatible "$tmp_dir/old.api" "$tmp_dir/new.api"
