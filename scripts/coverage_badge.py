#!/usr/bin/env python3
"""Render a GitHub-hosted badge from the measured Go statement coverage."""
import argparse
import html
import json
import re
from collections import defaultdict
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path


def read_coverage(profile: Path) -> dict[str, tuple[int, int]]:
    lines = profile.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0] not in {"mode: set", "mode: count", "mode: atomic"}:
        raise ValueError("missing or invalid Go coverage mode")
    blocks: dict[str, tuple[int, int]] = {}
    for line in lines[1:]:
        location, statements, hits = line.rsplit(maxsplit=2)
        statements, hits = int(statements), int(hits)
        if statements < 0 or hits < 0 or ":" not in location:
            raise ValueError("invalid coverage block")
        previous = blocks.get(location, (statements, 0))
        if previous[0] != statements:
            raise ValueError("inconsistent statement count for duplicate block")
        blocks[location] = (statements, previous[1] + hits)
    packages = defaultdict(lambda: [0, 0])
    for location, (statements, hits) in blocks.items():
        package = location.rsplit("/", 1)[0]
        packages[package][0] += statements if hits else 0
        packages[package][1] += statements
    if sum(total for _, total in packages.values()) == 0:
        raise ValueError("coverage profile has no instrumented statements")
    return {name: tuple(counts) for name, counts in sorted(packages.items())}


def percent(covered: int, total: int) -> str:
    value = Decimal(covered) * 100 / Decimal(total)
    return str(value.quantize(Decimal("0.1"), rounding=ROUND_HALF_UP))


def render_svg(covered: int, total: int) -> str:
    value = percent(covered, total) + "%"
    ratio = Decimal(covered) * 100 / Decimal(total)
    color = "#4c1" if ratio >= 90 else "#97ca00" if ratio >= 80 else "#dfb317" if ratio >= 60 else "#e05d44"
    label = html.escape(f"coverage: {value}", quote=True)
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="116" height="20" role="img" aria-label="{label}">
  <title>{label}</title>
  <linearGradient id="shade" x2="0" y2="100%">
    <stop offset="0" stop-color="#fff" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="clip"><rect width="116" height="20" rx="3"/></clipPath>
  <g clip-path="url(#clip)">
    <path fill="#555" d="M0 0h63v20H0z"/>
    <path fill="{color}" d="M63 0h53v20H63z"/>
    <path fill="url(#shade)" d="M0 0h116v20H0z"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
    <text x="31.5" y="15" fill="#010101" fill-opacity=".3">coverage</text>
    <text x="31.5" y="14">coverage</text>
    <text x="89.5" y="15" fill="#010101" fill-opacity=".3">{value}</text>
    <text x="89.5" y="14">{value}</text>
  </g>
</svg>
'''


def write_report(profile: Path, output: Path, repository: str, sha: str, run_id: str) -> None:
    if not re.fullmatch(r"[\w.-]+/[\w.-]+", repository):
        raise ValueError("expected owner/repository")
    if not re.fullmatch(r"[0-9a-f]{40}", sha) or not re.fullmatch(r"[0-9]+", run_id):
        raise ValueError("expected a commit SHA and numeric workflow run ID")
    packages = read_coverage(profile)
    covered = sum(pair[0] for pair in packages.values())
    total = sum(pair[1] for pair in packages.values())
    run_url = f"https://github.com/{repository}/actions/runs/{run_id}"
    commit_url = f"https://github.com/{repository}/commit/{sha}"
    result = {"coverage": percent(covered, total), "covered_statements": covered,
              "total_statements": total, "source_commit": sha, "workflow_run": run_url,
              "packages": {name: {"covered": c, "total": t} for name, (c, t) in packages.items()}}
    output.mkdir(parents=True, exist_ok=True)
    (output / "coverage.svg").write_text(render_svg(covered, total), encoding="utf-8")
    (output / "summary.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    lines = ["# Measured statement coverage", "", "![Coverage](coverage.svg)", "",
             f"Source: [{sha[:12]}]({commit_url}) · [GitHub Actions run]({run_url})", "",
             f"**{percent(covered, total)}%** — {covered} of {total} instrumented statements covered.", "",
             "Generated from the CI coverage profile, not a manually maintained percentage.",
             "Scope: the library packages and shared internal implementations selected by",
             "<code>scripts/check-library-coverage.sh</code>. This is statement coverage, not line coverage.", "",
             "| Package | Statement coverage |", "| --- | ---: |"]
    for name, (c, t) in packages.items():
        value = percent(c, t) + "%" if t else "No statements"
        lines.append(f"| <code>{html.escape(name)}</code> | {value} ({c}/{t}) |")
    (output / "README.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"Measured coverage: {percent(covered, total)}% ({covered}/{total})")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profile", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    try:
        write_report(args.profile, args.output, args.repository, args.sha, args.run_id)
    except (OSError, ValueError) as exc:
        parser.error(str(exc))


if __name__ == "__main__":
    main()
