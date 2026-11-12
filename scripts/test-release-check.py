#!/usr/bin/env python3
"""Tests for the pure checks in scripts/release-check.py.

  python3 scripts/test-release-check.py
"""
import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("release_check", HERE / "release-check.py")
rc = importlib.util.module_from_spec(spec)
sys.modules["release_check"] = rc
spec.loader.exec_module(rc)


class ReleaseCheckTest(unittest.TestCase):
    def test_notes(self):
        self.assertEqual(rc.notes_problems("## Highlights\n\n- x\n", "0.7.0"), [])
        self.assertIn("does not exist", rc.notes_problems(None, "0.7.0")[0])
        self.assertIn("Planned", rc.notes_problems("- x\n\n## Planned for this train (proposed)\n", "0.7.0")[0])
        self.assertTrue(rc.notes_problems("", "0.7.0"))
        # The word in prose is fine; only a heading counts.
        self.assertEqual(rc.notes_problems("- Ships what was planned.\n", "0.7.0"), [])

    def test_docs_pin(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            (d / "install.mdx").write_text(
                "Install the released 0.7.0 formula.\n"
                "[kit_0.7.0_windows_amd64.zip](https://github.com/hev/kit/releases/download/v0.7.0/kit_0.7.0_windows_amd64.zip)\n"
                "go install github.com/hev/kit/cmd/hev@v0.7.0\n"
                "ssh bot@100.64.0.10 and Go 1.27\n")
            (d / "orchestrators.mdx").write_text("In 0.6.1, expedite changed.\n")  # history, not pinned
            self.assertEqual(rc.docs_pin_problems(d, "0.7.0"), [])
            self.assertEqual(rc.docs_pin_problems(d, "0.7.1")[0], "install.mdx:1 names 0.7.0")
            (d / "windows.mdx").write_text("Download kit_0.6.2_windows_amd64.zip\n")
            self.assertEqual(rc.docs_pin_problems(d, "0.7.0"), ["windows.mdx:1 names 0.6.2"])

    def test_changelog(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            self.assertIn("missing", rc.changelog_problem(d, "0.7.0"))
            (d / "changelog.mdx").write_text("## 0.7.0\n\nNew.\n\n## 0.6.2\n")
            self.assertIsNone(rc.changelog_problem(d, "0.7.0"))
            self.assertIn("0.7.1", rc.changelog_problem(d, "0.7.1"))

    def test_marker_matches_generator(self):
        sha = "a" * 40
        body = f"x\n<!-- kit-release-draft version=0.7.0 base=v0.6.2 source={sha} -->\n"
        self.assertEqual(rc.marker(body), {"version": "0.7.0", "base": "v0.6.2", "source": sha})
        self.assertIsNone(rc.marker("no marker"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
