#!/usr/bin/env bash
set -euo pipefail

# Explicit developer action: may download the pinned module and its dependencies.
# Never creates a go.mod or go.work in the caller's project.
skill_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
module='github.com/Yakwilik/go-yamlvalidator'
version='v1.0.0'
local_repo=''
test_args=(-count=1)
while (($#)); do
  case "$1" in
    --local)
      if (($# < 2)); then echo '--local requires a repository path' >&2; exit 2; fi
      local_repo="$(cd -- "$2" && pwd)"
      shift 2
      ;;
    --race) test_args+=(-race); shift ;;
    -h|--help)
      echo 'Usage: check-examples.sh [--local /path/to/go-yamlvalidator] [--race]'
      exit 0
      ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done
command -v go >/dev/null || { echo 'Go 1.24+ is required' >&2; exit 2; }
export GOWORK=off
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
if [[ -n "$local_repo" ]]; then
  actual_module="$(cd -- "$local_repo" && go list -m -f '{{.Path}}')"
  if [[ "$actual_module" != "$module" ]]; then
    echo "--local must point to $module, got $actual_module" >&2
    exit 2
  fi
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/yamlvalidator-skill.XXXXXXXX")"
trap 'rm -rf -- "$tmp_dir"' EXIT
cp -R -- "$skill_dir/examples" "$tmp_dir/examples"
cd -- "$tmp_dir"
go mod init example.com/yamlvalidator-skill-check
go mod edit -go=1.24.0 -require="${module}@${version}"
if [[ -n "$local_repo" ]]; then
  go mod edit -replace="${module}=${local_repo}"
fi
go mod tidy
go test "${test_args[@]}" ./examples/...
go run ./examples/native
go run ./examples/jsonschema
