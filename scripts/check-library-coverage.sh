#!/usr/bin/env bash
set -euo pipefail

minimum="${COVERAGE_MINIMUM:-80.0}"
output="${1:-coverage.out}"
# Measure implementations as well as their public facades. Moving code into
# internal must not turn the gate into a check of forwarding wrappers alone.
packages=(
  "."
  "./internal/engine"
  "./internal/yamlcodec"
  "./internal/genspec"
  "./internal/schemacompiler"
  "./genruntime"
  "./genruntime/spec"
  "./pkg/valuevalidator"
  "./pkg/keyvalidator"
)
cover_packages="$(IFS=,; echo "${packages[*]}")"
module_path="$(go list -m -f '{{.Path}}')"
go test -count=1 -covermode=atomic -coverpkg="$cover_packages" -coverprofile="$output" ./...
python3 - "$output" "$minimum" "$module_path" "${packages[@]}" <<'PY'
from collections import defaultdict
from decimal import Decimal
from pathlib import Path
import sys

path, minimum, module, *packages = sys.argv[1:]
minimum = Decimal(minimum)
if not Decimal(0) <= minimum <= Decimal(100):
    raise SystemExit('COVERAGE_MINIMUM must be in [0, 100]')
# go test -coverpkg may emit the same block from multiple test binaries.
# Merge hits per source block, just as the Go profile parser does.
blocks = {}
for line in Path(path).read_text().splitlines()[1:]:
    location, count, hits = line.split()
    count, hits = int(count), int(hits)
    previous = blocks.get(location, (count, 0))
    if previous[0] != count:
        raise SystemExit(f'inconsistent coverage block {location}')
    blocks[location] = (count, previous[1] + hits)
totals = defaultdict(lambda: [0, 0])
for location, (count, hits) in blocks.items():
    package = location.rsplit('/', 1)[0]
    totals[package][0] += count
    totals[package][1] += count if hits else 0
failed = False
for package in packages:
    full = module if package == '.' else module + '/' + package.removeprefix('./')
    total, covered = totals[full]
    if total == 0:
        print(f'{package}: no instrumented statements', file=sys.stderr)
        failed = True
        continue
    percent = Decimal(covered) * 100 / Decimal(total)
    print(f'{package} coverage: {percent:.2f}% ({covered}/{total})')
    if percent < minimum:
        print(f'coverage for {package} is below {minimum}%', file=sys.stderr)
        failed = True
if failed:
    raise SystemExit(1)
# go test -coverpkg emits the same source block from several test binaries.
# Publish one canonical entry per block, with merged hit counts, rather than
# making external reporters interpret duplicate source ranges themselves.
# This preserves exactly the coverage already measured by the gate above.
normalized = ["mode: atomic"]
normalized.extend(
    f"{location} {count} {hits}"
    for location, (count, hits) in sorted(blocks.items())
)
Path(path).write_text("\n".join(normalized) + "\n")
PY
go tool cover -func="$output" | tail -1
