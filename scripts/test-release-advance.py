#!/usr/bin/env python3
"""Tests for scripts/release-advance.py against a scratch bare "origin".

  python3 scripts/test-release-advance.py
"""
import importlib.util
import io
import os
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("release_advance", HERE / "release-advance.py")
ra = importlib.util.module_from_spec(spec)
sys.modules["release_advance"] = ra
spec.loader.exec_module(ra)

ENV = {**os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
       "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
       "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}


class AdvanceTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="release-advance-test-")
        root = Path(self.tmp.name)
        self.origin, self.work = root / "origin.git", root / "work"
        subprocess.check_call(["git", "init", "-q", "--bare", "-b", "main", str(self.origin)], env=ENV)
        subprocess.check_call(["git", "clone", "-q", str(self.origin), str(self.work)], env=ENV,
                              stderr=subprocess.DEVNULL)
        self.git("checkout", "-q", "-b", "main")
        self.commit("v0.6.2")
        self.git("tag", "v0.6.2")
        self.git("push", "-q", "origin", "main", "v0.6.2")
        # The 0.7 train: v0.7 from main; a feature merges into it.
        self.git("checkout", "-q", "-b", "v0.7")
        self.commit("feature")
        self.git("push", "-q", "origin", "v0.7")

    def tearDown(self):
        self.tmp.cleanup()

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.work), *args], env=ENV, text=True,
                                       stderr=subprocess.PIPE).strip()

    def commit(self, msg):
        self.git("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", msg)
        return self.git("rev-parse", "HEAD")

    def publish(self, tag, ref="v0.7"):
        # Publishing the draft creates the tag on origin at the train branch commit.
        self.git("tag", tag, ref)
        self.git("push", "-q", "origin", tag)

    def remote(self, ref):
        out = subprocess.check_output(["git", "-C", str(self.origin), "rev-parse", "--verify", "--quiet", ref],
                                      env=ENV, text=True)
        return out.strip()

    def run_advance(self, *args):
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            rc = ra.main([*args, "--repo-dir", str(self.work)])
        return rc, out.getvalue() + err.getvalue()

    def test_dry_run_changes_nothing(self):
        self.publish("v0.7.0")
        main_before = self.remote("main")
        rc, out = self.run_advance("0.7.0")
        self.assertEqual(rc, 0, out)
        self.assertIn("would run: git push origin", out)
        self.assertEqual(self.remote("main"), main_before)
        with self.assertRaises(subprocess.CalledProcessError):
            self.remote("v0.8")

    def test_minor_fast_forwards_main_and_opens_next_train(self):
        self.publish("v0.7.0")
        tag = self.git("rev-parse", "v0.7.0^{commit}")
        rc, out = self.run_advance("0.7.0", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.remote("main"), tag)
        self.assertEqual(self.remote("v0.8"), tag)
        # Idempotent: a re-run has nothing to push.
        rc, out = self.run_advance("0.7.0", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertNotIn("ran:", out)
        self.assertIn("already at v0.7.0", out)

    def test_main_that_moved_is_never_forced(self):
        self.git("checkout", "-q", "main")
        self.commit("stray commit on main")
        self.git("push", "-q", "origin", "main")
        stray = self.remote("main")
        self.publish("v0.7.0")
        rc, out = self.run_advance("0.7.0", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("STOP", out)
        self.assertIn("never forced", out)
        self.assertEqual(self.remote("main"), stray)

    def test_patch_on_newest_line_and_older_line(self):
        self.publish("v0.7.0")
        self.assertEqual(self.run_advance("0.7.0", "--apply")[0], 0)
        # 0.8 work lands on v0.8; a fix lands on v0.7 and ships as 0.7.1.
        self.git("fetch", "-q", "origin")
        self.git("checkout", "-q", "-b", "v0.8", "origin/v0.8")
        self.commit("0.8 feature")
        self.git("push", "-q", "origin", "v0.8")
        self.git("checkout", "-q", "v0.7")
        self.commit("fix")
        self.git("push", "-q", "origin", "v0.7")
        self.publish("v0.7.1")
        rc, out = self.run_advance("0.7.1", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.remote("main"), self.git("rev-parse", "v0.7.1^{commit}"))
        self.assertIn("WARNING: train branch v0.8 does not contain v0.7.1", out)
        # A patch on an older line leaves main alone.
        self.git("checkout", "-q", "-b", "v0.6", "v0.6.2")
        self.commit("old fix")
        self.git("push", "-q", "origin", "v0.6")
        self.publish("v0.6.3", "v0.6")
        main = self.remote("main")
        rc, out = self.run_advance("0.6.3", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("not the newest release", out)
        self.assertEqual(self.remote("main"), main)

    def test_missing_tag_stops(self):
        rc, out = self.run_advance("0.9.0")
        self.assertEqual(rc, 1)
        self.assertIn("tag v0.9.0 not found", out)


if __name__ == "__main__":
    unittest.main(verbosity=2)
