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

# apidiff treats public type aliases whose canonical definitions moved into
# internal/engine as identity changes. v1.1 intentionally accepts that
# reflection-only relocation while preserving the application-facing root names
# and source signatures. Ignore only reports whose old/new type text becomes
# identical after canonicalizing this exact internal/engine relocation.
"$apidiff" -m -incompatible "$tmp_dir/old.api" "$tmp_dir/new.api" > "$tmp_dir/raw.txt"

python3 - "$module_path" "$tmp_dir/raw.txt" "$tmp_dir/remaining.txt" <<'PY'
import re
import sys
from pathlib import Path

module, raw_path, remaining_path = sys.argv[1:]
raw = Path(raw_path).read_text()
remaining = []
ignored = []

root_prefix = module + "."
engine_prefix = module + "/internal/engine."

def canon(text: str) -> str:
    text = text.replace(engine_prefix, root_prefix)
    text = text.replace(root_prefix, "")
    return text

for line in raw.splitlines():
    match = re.match(r"^- (.*?): changed from (.*) to (.*)$", line)
    if match and canon(match.group(2)) == canon(match.group(3)):
        ignored.append(line)
        continue
    if line.strip():
        remaining.append(line)

if ignored:
    print(f"apidiff: accepted {len(ignored)} canonical internal/engine alias relocations")
Path(remaining_path).write_text("\n".join(remaining) + ("\n" if remaining else ""))
PY

cat "$tmp_dir/remaining.txt"
if [[ -s "$tmp_dir/remaining.txt" ]]; then
  echo "API compatibility differences require review" >&2
  exit 1
fi
