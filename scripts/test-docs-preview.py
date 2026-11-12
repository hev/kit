#!/usr/bin/env python3
"""Tests for scripts/docs-preview.sh, with a stub pnpm (no Node needed).

  python3 scripts/test-docs-preview.py

Covers what the train relies on: build swaps `current` only after a build;
refresh follows a train docs branch, rebuilds after it is rebased, and falls
back to origin/main once the branch is deleted; serve answers on a MagicDNS
Host header (astro preview's host check answers 403 there).
"""
import http.client
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import unittest

SCRIPT = Path(__file__).resolve().parent / "docs-preview.sh"
MAGICDNS = "adams-mac-mini.tail07c773.ts.net"

STUB_PNPM = """#!/bin/sh
# Stub pnpm: `install` does nothing; `build` copies the docs pages into dist/.
case "$1" in
  install) exit 0 ;;
  build) mkdir -p dist/docs && cat src/content/docs/*.mdx > dist/docs/index.html ;;
  *) echo "stub pnpm: $*" >&2; exit 1 ;;
esac
"""

GIT_ENV = {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
           "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
           "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class DocsPreviewTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="docs-preview-test-")
        t = Path(self.tmp.name)
        bin_dir = t / "bin"
        bin_dir.mkdir()
        (bin_dir / "pnpm").write_text(STUB_PNPM)
        (bin_dir / "pnpm").chmod(0o755)
        self.env = {**os.environ, **GIT_ENV, "PATH": f"{bin_dir}:{os.environ['PATH']}",
                    "KIT_PREVIEW_DIR": str(t / "state"), "KIT_PREVIEW_NAME": MAGICDNS}
        self.origin = t / "origin.git"
        self.work = t / "work"
        self.clone = t / "clone"
        subprocess.run(["git", "init", "-q", "--bare", "-b", "main", str(self.origin)], check=True, env=self.env)
        subprocess.run(["git", "clone", "-q", str(self.origin), str(self.work)], check=True, env=self.env,
                       stderr=subprocess.DEVNULL)
        self.page("index", "main docs\n", "site onto main")
        self.git(self.work, "push", "-q", "origin", "HEAD:main")
        self.git(self.work, "checkout", "-q", "-b", "train/v0.7-docs")
        self.page("context", "Coming in 0.7\n", "train docs")
        self.git(self.work, "push", "-q", "origin", "train/v0.7-docs")
        self.git(self.work, "checkout", "-q", "main")
        subprocess.run(["git", "clone", "-q", str(self.origin), str(self.clone)], check=True, env=self.env)
        self.env["KIT_PREVIEW_REPO"] = str(self.clone)

    def tearDown(self):
        self.tmp.cleanup()

    def git(self, cwd, *args):
        return subprocess.run(["git", "-C", str(cwd), *args], check=True, env=self.env,
                              capture_output=True, text=True).stdout.strip()

    def page(self, slug, text, msg):
        site = self.work / "site"
        (site / "src/content/docs").mkdir(parents=True, exist_ok=True)
        (site / "package.json").write_text("{}\n")
        (site / f"src/content/docs/{slug}.mdx").write_text(text)
        self.git(self.work, "add", "-A")
        self.git(self.work, "-c", "commit.gpgsign=false", "commit", "-q", "-m", msg)

    def preview(self, *args, ref=None):
        env = dict(self.env)
        if ref:
            env["KIT_PREVIEW_REF"] = ref
        return subprocess.run(["bash", str(SCRIPT), *args], env=env, capture_output=True, text=True)

    def current(self):
        state = Path(self.env["KIT_PREVIEW_DIR"]) / "current"
        return json.loads((state / "preview.json").read_text()), (state / "docs/index.html").read_text()

    def test_refresh_follows_train_branch_then_falls_back_to_main(self):
        train = "origin/train/v0.7-docs"
        p = self.preview("refresh", ref=train)
        self.assertEqual(p.returncode, 0, p.stderr)
        meta, html = self.current()
        self.assertEqual(meta["ref"], train)
        self.assertIn("Coming in 0.7", html)
        # Idle tick: nothing moved, nothing built.
        p = self.preview("refresh", ref=train)
        self.assertIn("up to date", p.stderr)
        # A feature merges with its page; the train docs branch is rebased
        # (force-pushed) on main. Refresh picks up the new tip.
        self.page("context", "Context, shipped\n", "context lands (#220)")
        self.git(self.work, "push", "-q", "origin", "main")
        self.git(self.work, "push", "-q", "origin", "--force", "main:train/v0.7-docs")
        p = self.preview("refresh", ref=train)
        self.assertEqual(p.returncode, 0, p.stderr)
        meta, html = self.current()
        self.assertEqual(meta["sha"], self.git(self.work, "rev-parse", "main"))
        self.assertIn("Context, shipped", html)
        # The cut: the train docs branch is deleted; the preview follows main.
        self.git(self.work, "push", "-q", "origin", "--delete", "train/v0.7-docs")
        self.page("index", "main docs, cut\n", "0.7 changelog")
        self.git(self.work, "push", "-q", "origin", "main")
        p = self.preview("refresh", ref=train)
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertIn("following origin/main", p.stderr)
        meta, html = self.current()
        self.assertEqual(meta["ref"], "origin/main")
        self.assertIn("main docs, cut", html)

    def test_serve_answers_on_magicdns_host(self):
        self.assertEqual(self.preview("build", "origin/main").returncode, 0)
        port = free_port()
        env = {**self.env, "KIT_PREVIEW_PORT": str(port)}
        proc = subprocess.Popen(["bash", str(SCRIPT), "serve"], env=env,
                                stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
        try:
            for _ in range(50):
                try:
                    socket.create_connection(("127.0.0.1", port), timeout=0.2).close()
                    break
                except OSError:
                    time.sleep(0.1)
            for host in (MAGICDNS, f"{MAGICDNS}:{port}", "100.126.12.95", f"127.0.0.1:{port}"):
                c = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
                c.request("GET", "/docs/", headers={"Host": host})
                r = c.getresponse()
                body = r.read().decode()
                c.close()
                self.assertEqual(r.status, 200, host)
                self.assertIn("main docs", body)
        finally:
            proc.terminate()
            _, err = proc.communicate(timeout=5)
        self.assertIn(f"http://{MAGICDNS}:{port}/docs", err)


if __name__ == "__main__":
    unittest.main(verbosity=2)
