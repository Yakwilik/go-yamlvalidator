"""Regression tests for measured (not hard-coded) badge percentages."""
import json
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

from coverage_badge import percent, read_coverage, render_svg, write_report


class CoverageBadgeTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.profile = self.root / "coverage.out"

    def profile_text(self, text):
        self.profile.write_text(text, encoding="utf-8")
        return read_coverage(self.profile)

    def test_weighted_statements_and_duplicate_blocks(self):
        result = self.profile_text("mode: atomic\nm/p/a.go:1.1,2.2 9 0\n"
                                   "m/p/a.go:1.1,2.2 9 2\nm/p/a.go:3.1,4.2 1 0\n")
        self.assertEqual(result, {"m/p": (9, 10)})
        self.assertEqual(percent(*result["m/p"]), "90.0")

    def test_bad_or_empty_profile_fails(self):
        for text in ("", "mode: atomic\n", "mode: wrong\nm/p/a.go:1.1,2.2 2 1\n",
                     "mode: set\nm/p/a.go:1.1,2.2 2 -1\n",
                     "mode: set\nm/p/a.go:1.1,2.2 2 1\nm/p/a.go:1.1,2.2 3 1\n"):
            with self.subTest(text=text), self.assertRaises(ValueError):
                self.profile_text(text)

    def test_svg_zero_full_and_threshold(self):
        for covered in (0, 79, 80, 90, 100):
            svg = ET.fromstring(render_svg(covered, 100))
            self.assertEqual(svg.attrib["aria-label"], f"coverage: {covered}.0%")
        self.assertIn('#e05d44', render_svg(0, 100))
        self.assertIn('#97ca00', render_svg(80, 100))
        self.assertIn('#4c1', render_svg(100, 100))

    def test_report_matches_profile_and_identifies_source(self):
        self.profile_text("mode: count\nm/p/a.go:1.1,2.2 816 1\n"
                          "m/p/a.go:3.1,4.2 184 0\n")
        output = self.root / "report"
        write_report(self.profile, output, "owner/repo", "a" * 40, "123")
        data = json.loads((output / "summary.json").read_text())
        self.assertEqual(data["coverage"], "81.6")
        self.assertEqual(data["covered_statements"], 816)
        self.assertEqual(data["source_commit"], "a" * 40)
        self.assertIn("actions/runs/123", (output / "README.md").read_text())
        self.assertIn("81.6%", (output / "coverage.svg").read_text())

    def test_invalid_source_metadata_fails(self):
        with self.assertRaises(ValueError):
            write_report(self.profile, self.root / "out", "bad/repo/extra", "no", "no")


if __name__ == "__main__":
    unittest.main()
