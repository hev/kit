#!/usr/bin/env python3
"""Tests for the pure checks in scripts/release-check.py.

  python3 scripts/test-release-check.py
"""
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("release_check", HERE / "release-check.py")
rc = importlib.util.module_from_spec(spec)
sys.modules["release_check"] = rc
spec.loader.exec_module(rc)
spec = importlib.util.spec_from_file_location("release_draft", HERE / "release-draft.py")
rd = importlib.util.module_from_spec(spec)
sys.modules["release_draft"] = rd
spec.loader.exec_module(rd)


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

    def test_train_docs_branch(self):
        self.assertEqual(rc.train_docs_branch(None, "0.8.0"), "train/v0.8-docs")
        self.assertEqual(rc.train_docs_branch("# 0.7\n<!-- train-docs: d/kit-docs-07-20261111 -->\n", "0.7.0"),
                         "d/kit-docs-07-20261111")
        self.assertEqual(rc.train_docs_branch(None, "0.7.0"), rd.train_docs_branch(None, "0.7.0"))

    def test_train_branch(self):
        self.assertEqual(rc.train_branch("0.7.0"), "v0.7")
        self.assertEqual(rc.train_branch("0.7.3"), "v0.7")
        self.assertEqual(rd.train_line(rc.train_branch("1.12.0")), (1, 12))

    def test_lineage(self):
        env = {**os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
               "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
               "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}
        with tempfile.TemporaryDirectory() as tmp:
            def git(*a):
                return subprocess.check_output(["git", "-C", tmp, *a], env=env, text=True).strip()

            def commit(msg):
                git("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", msg)
                return git("rev-parse", "HEAD")

            git("init", "-q", "-b", "main")
            commit("v0.6.2")
            git("checkout", "-q", "-b", "v0.7")
            head = commit("feature")
            d = Path(tmp)
            self.assertEqual(rc.lineage_problems(d, head, "v0.7", "main"), [])
            self.assertIn("does not resolve", rc.lineage_problems(d, head, "v0.8", "main")[0])
            # A commit that is not on the train branch.
            git("checkout", "-q", "-b", "side", "main")
            side = commit("elsewhere")
            self.assertIn("not on v0.7", rc.lineage_problems(d, side, "v0.7", "main")[0])
            # main moved after the train branch opened (a patch, or a stray
            # merge): it cannot be fast-forwarded to the tag.
            git("checkout", "-q", "main")
            commit("stray commit on main")
            problems = rc.lineage_problems(d, head, "v0.7", "main")
            self.assertEqual(len(problems), 1)
            self.assertIn("not an ancestor", problems[0])
            # Merging main into the train branch repairs it, without forcing main.
            git("checkout", "-q", "v0.7")
            git("-c", "commit.gpgsign=false", "merge", "-q", "--no-ff", "main", "-m", "Merge main into v0.7")
            self.assertEqual(rc.lineage_problems(d, git("rev-parse", "HEAD"), "v0.7", "main"), [])

    def test_dependabot_targets_next_train(self):
        two = ('version: 2\nupdates:\n  - package-ecosystem: "gomod"\n    directory: "/"\n'
               '    target-branch: "v0.8"\n  - package-ecosystem: "docker"\n    target-branch: v0.8\n')
        self.assertEqual(rc.dependabot_problems(two, "0.7.0"), [])
        self.assertEqual(rc.dependabot_problems(two, "0.7.1"), [])  # patches leave it alone
        self.assertEqual(rc.dependabot_problems(two, "0.8.0"), ["target-branch v0.8 (want v0.9)"] * 2)
        none = 'version: 2\nupdates:\n  - package-ecosystem: "gomod"\n    directory: "/"\n'
        self.assertIn("no target-branch", rc.dependabot_problems(none, "0.7.0")[0])

    def test_marker_matches_generator(self):
        sha = "a" * 40
        body = f"x\n<!-- kit-release-draft version=0.7.0 base=v0.6.2 source={sha} -->\n"
        self.assertEqual(rc.marker(body), {"version": "0.7.0", "base": "v0.6.2", "source": sha})
        self.assertIsNone(rc.marker("no marker"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
