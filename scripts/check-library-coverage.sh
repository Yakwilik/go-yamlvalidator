#!/usr/bin/env bash
set -euo pipefail

minimum="${COVERAGE_MINIMUM:-80.0}"
output="${1:-coverage.out}"
packages=(
  "."
  "./pkg/valuevalidator"
  "./pkg/keyvalidator"
)

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/yamlvalidator-coverage.XXXXXXXX")"
trap 'rm -rf -- "$tmp_dir"' EXIT
for pkg in "${packages[@]}"; do
  safe_name="$(printf '%s' "$pkg" | tr '/.' '__')"
  profile="$tmp_dir/${safe_name}.out"
  go test "$pkg" -covermode=atomic -coverprofile="$profile"
  coverage="$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
  printf '%s coverage: %s%%\n' "$pkg" "$coverage"
  if ! awk -v coverage="$coverage" -v minimum="$minimum" 'BEGIN { exit !(coverage + 0 >= minimum + 0) }'; then
    printf 'coverage for %s is below %s%%\n' "$pkg" "$minimum" >&2
    exit 1
  fi
done

go test "${packages[@]}" -covermode=atomic -coverprofile="$output"
go tool cover -func="$output" | tail -1
