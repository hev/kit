#!/usr/bin/env python3
"""Tests for scripts/edge-build.py: the edge version, the manifest, pruning,
and that edge tags stay invisible to the release scripts.

  python3 scripts/test-edge-build.py
"""
import contextlib
import hashlib
import io
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("edge_build", HERE / "edge-build.py")
eb = importlib.util.module_from_spec(spec)
sys.modules["edge_build"] = eb
spec.loader.exec_module(eb)

ENV = {**os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
       "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
       "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}
SHA = "0123456789abcdef0123456789abcdef01234567"
V = "0.7.0-edge.0123456"


def fake_dist(root: Path, version: str = V, skip=None, corrupt=None) -> Path:
    dist = root / "dist"
    dist.mkdir()
    lines = []
    for os_, arch in sorted(eb.PLATFORMS):
        if (os_, arch) == skip:
            continue
        name = f"kit_{version}_{os_}_{arch}." + ("zip" if os_ == "windows" else "tar.gz")
        body = f"archive {os_} {arch}".encode()
        (dist / name).write_bytes(body)
        digest = hashlib.sha256(body).hexdigest()
        if name == corrupt:
            digest = "0" * 64
        lines.append(f"{digest}  {name}")
    (dist / "checksums.txt").write_text("\n".join(lines) + "\n")
    (dist / "unrelated.txt").write_text("not an asset")
    return dist


class ManifestTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="edge-build-test-")
        self.root = Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def test_manifest_names_version_commit_and_every_digest(self):
        dist = fake_dist(self.root)
        m = eb.manifest(dist, V, "v0.7", SHA, 42, built="2026-11-12T00:00:00Z")
        self.assertEqual((m["schema"], m["version"], m["channel"], m["train"], m["commit"], m["sequence"]),
                         (1, V, "edge", "v0.7", SHA, 42))
        self.assertEqual({(a["os"], a["arch"]) for a in m["assets"]}, eb.PLATFORMS)
        for a in m["assets"]:
            body = (dist / a["name"]).read_bytes()
            self.assertEqual(a["sha256"], hashlib.sha256(body).hexdigest())
            self.assertEqual(a["size"], len(body))
        # The CLI writes it beside the archives, where publish picks it up.
        with contextlib.redirect_stdout(io.StringIO()):
            rc = eb.main(["manifest", "--dist", str(dist), "--version", V, "--train", "v0.7",
                          "--commit", SHA, "--sequence", "42"])
        self.assertEqual(rc, 0)
        self.assertEqual(json.loads((dist / "manifest.json").read_text())["version"], V)

    def test_manifest_refuses_inconsistent_dist(self):
        with self.assertRaisesRegex(eb.EdgeError, "checksums.txt says"):
            eb.manifest(fake_dist(self.root, corrupt=f"kit_{V}_linux_amd64.tar.gz"), V, "v0.7", SHA, 1)
        other = self.root / "o"
        other.mkdir()
        with self.assertRaisesRegex(eb.EdgeError, "want"):
            eb.manifest(fake_dist(other, skip=("windows", "amd64")), V, "v0.7", SHA, 1)
        third = self.root / "t"
        third.mkdir()
        with self.assertRaisesRegex(eb.EdgeError, "not built as"):
            eb.manifest(fake_dist(third, version="0.7.0-edge.fffffff"), V, "v0.7", SHA, 1)
        for bad in [("0.7.0", "v0.7", SHA), (V, "main", SHA), (V, "v0.7", "abc")]:
            with self.assertRaises(eb.EdgeError):
                eb.manifest(fake_dist(Path(tempfile.mkdtemp(dir=self.root))), *bad, 1)


def rel(tag, at, prerelease=True, draft=False):
    return {"tag_name": tag, "published_at": at, "prerelease": prerelease, "draft": draft}


class PruneTest(unittest.TestCase):
    def test_keeps_newest_edge_builds_of_the_train_only(self):
        edges = [rel(f"edge-0.7.0-edge.{i:07x}", f"2026-11-{i + 1:02d}T00:00:00Z") for i in range(12)]
        others = [
            rel("v0.6.2", "2026-10-01T00:00:00Z", prerelease=False),          # a release
            rel("v0.7.0", "", prerelease=False, draft=True),                   # the train's draft
            rel("edge-0.8.0-edge.aaaaaaa", "2026-10-02T00:00:00Z"),            # another train
            rel("edge-0.7.0-edge.bbbbbbb", "2026-10-03T00:00:00Z", prerelease=False),  # not a pre-release
            rel("edge-0.7.0-edge.ccccccc", "", draft=True),                    # a half-published draft
            rel("v0.7.0-edge.ddddddd", "2026-10-04T00:00:00Z"),                # not our tag shape
        ]
        doomed = eb.plan_prune(others + edges, "v0.7", 10)
        self.assertEqual(doomed, ["edge-0.7.0-edge.0000001", "edge-0.7.0-edge.0000000"])
        self.assertEqual(eb.plan_prune(others + edges, "v0.7", 20), [])
        self.assertEqual(eb.plan_prune(others + edges, "v0.8", 1), [])
        # A patch line's edge builds (0.7.1-edge) belong to the same train.
        self.assertEqual(eb.plan_prune([rel("edge-0.7.1-edge.eeeeeee", "2026-12-01T00:00:00Z")] + edges, "v0.7", 1),
                         [r["tag_name"] for r in reversed(edges)])
        with self.assertRaises(eb.EdgeError):
            eb.plan_prune(edges, "v0.7", 0)
        with self.assertRaises(eb.EdgeError):
            eb.plan_prune(edges, "main", 3)


class VersionAndTagTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="edge-build-git-")
        self.work = Path(self.tmp.name)
        self.git("init", "-q", "-b", "main")
        self.git("commit", "-q", "--allow-empty", "-m", "release")
        self.git("tag", "v0.6.2")
        self.git("checkout", "-q", "-b", "v0.7")
        self.git("commit", "-q", "--allow-empty", "-m", "feature")

    def tearDown(self):
        self.tmp.cleanup()

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.work), *args], env=ENV, text=True).strip()

    def test_version_is_the_drafts_version_plus_the_commit(self):
        short = self.git("rev-parse", "--short=7", "HEAD")
        self.assertEqual(eb.edge_version(str(self.work), "v0.7"), f"0.7.0-edge.{short}")
        with self.assertRaises(eb.EdgeError):
            eb.edge_version(str(self.work), "main")

    def test_edge_tags_are_invisible_to_the_release_scripts(self):
        # release-draft.py finds the last release with this glob; an edge tag
        # on a later commit must not become the base of the next draft.
        sys.path.insert(0, str(HERE))
        spec = importlib.util.spec_from_file_location("release_draft", HERE / "release-draft.py")
        rd = importlib.util.module_from_spec(spec)
        sys.modules["release_draft"] = rd
        spec.loader.exec_module(rd)
        self.git("tag", f"edge-0.7.0-edge.{self.git('rev-parse', '--short=7', 'HEAD')}")
        out = self.git("describe", "--tags", "--abbrev=0", "--match", rd.TAG_GLOB, "HEAD")
        self.assertEqual(out, "v0.6.2")
        self.assertNotIn("edge", " ".join(self.git("tag", "-l", rd.TAG_GLOB).split()))
        # The Go updater and this script agree on the tag shape.
        self.assertTrue(eb.EDGE_TAG.match("edge-" + V))


if __name__ == "__main__":
    unittest.main()
