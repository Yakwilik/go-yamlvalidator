#!/usr/bin/env bash
set -euo pipefail

report="${1:?expected generated report directory}"
source_sha="${2:?expected measured master commit}"
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || { echo 'invalid source SHA' >&2; exit 1; }

# Publish only for the current master; an older CI run must not replace it.
current_master() { git ls-remote origin refs/heads/master | cut -f1; }
if [[ "$(current_master)" != "$source_sha" ]]; then
  echo 'Skipping coverage badge for an outdated master commit.'
  exit 0
fi
python3 - "$report/summary.json" "$source_sha" <<'PY'
import json, sys
from pathlib import Path
if json.loads(Path(sys.argv[1]).read_text())["source_commit"] != sys.argv[2]:
    raise SystemExit("coverage report does not match the measured commit")
PY

# A separate index avoids modifying the checkout or staging source files.
export GIT_INDEX_FILE
GIT_INDEX_FILE="$(mktemp)"
trap 'rm -f "$GIT_INDEX_FILE" "$GIT_INDEX_FILE.lock"' EXIT
rm -f "$GIT_INDEX_FILE"
parent="$(git ls-remote origin refs/heads/coverage | cut -f1)"
parents=()
if [[ -n "$parent" ]]; then
  git fetch --no-tags --depth=1 origin refs/heads/coverage
  parent="$(git rev-parse FETCH_HEAD)"
  git read-tree "$parent"
  parents=(-p "$parent")
else
  git read-tree --empty
fi
for file in coverage.svg summary.json README.md; do
  blob="$(git hash-object -w "$report/$file")"
  git update-index --add --cacheinfo "100644,$blob,$file"
done
tree="$(git write-tree)"
if [[ -n "$parent" && "$tree" == "$(git rev-parse "$parent^{tree}")" ]]; then
  echo 'Coverage badge is already current.'
  exit 0
fi
commit="$(printf 'coverage: update for %s\n' "$source_sha" |
  git -c user.name='github-actions[bot]' \
      -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
      commit-tree "$tree" "${parents[@]}")"
if [[ "$(current_master)" != "$source_sha" ]]; then
  echo 'Skipping badge because master advanced during publication.'
  exit 0
fi
# No force push: a concurrent update must fail rather than overwrite history.
# This branch has only reports, no workflow files or source-history bot commits.
git push origin "$commit:refs/heads/coverage"
