#!/usr/bin/env python3
"""Tests for scripts/edge-build.py: the edge version, the manifest, pruning,
the image tags, and that edge tags stay invisible to the release scripts.

  python3 scripts/test-edge-build.py
"""
import contextlib
import re
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


def heads(**trains):
    """git ls-remote output for train branches (and one non-train branch)."""
    lines = [f"{'f' * 40}\trefs/heads/main"]
    lines += [f"{sha}\trefs/heads/{name.replace('_', '.')}" for name, sha in trains.items()]
    return "\n".join(lines) + "\n"


OLD = "fedcba9876543210fedcba9876543210fedcba98"


class ImageTagsTest(unittest.TestCase):
    def test_tags_are_the_edge_version_and_edge_only(self):
        self.assertEqual(eb.image_tags(V), [f"hevlayer/kit:{V}", "hevlayer/kit:edge"])
        self.assertEqual(eb.image_tags(V, move_edge=False), [f"hevlayer/kit:{V}"])

    def test_never_latest_or_a_bare_version(self):
        for bad in ["0.7.0", "v0.7.0", "latest", "edge", "0.7.0-edge", "0.7.0-edge.XYZ1234", "0.7.0-rc.1",
                    f"{V}\nlatest", f"{V} latest",
                    # A trailing newline ($ matches before it) and non-ASCII digits (\\d matches them).
                    f"{V}\n", "0.\u0667.0-edge.0123456", "\uff10.7.0-edge.0123456"]:
            with self.subTest(version=bad):
                with self.assertRaises(eb.EdgeError):
                    eb.image_tags(bad)
                with self.assertRaises(eb.EdgeError):
                    eb.build_image_tags(bad, SHA, "v0.7", heads(v0_7=SHA))
                with contextlib.redirect_stderr(io.StringIO()), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(eb.main(["image-tags", "--version", bad, "--commit", SHA, "--train", "v0.7"]), 1)
        for move in (True, False):
            for t in eb.image_tags(V, move):
                tag = t.rsplit(":", 1)[1]
                self.assertNotEqual(tag, "latest")
                self.assertIsNone(re.fullmatch(r"v?\d+\.\d+\.\d+", tag))
                self.assertIn(tag, (V, "edge"))

    def test_version_must_be_built_from_the_commit(self):
        with self.assertRaisesRegex(eb.EdgeError, "not built from"):
            eb.build_image_tags(V, OLD, "v0.7", heads(v0_7=OLD))


class MovesEdgeTest(unittest.TestCase):
    """:edge moves only to the tip of the newest train, so it never moves back."""

    def tags(self, ls_remote, commit=SHA, train="v0.7"):
        return eb.build_image_tags(V, commit, train, ls_remote)[0]

    def test_tip_of_the_newest_train_moves_edge(self):
        self.assertEqual(self.tags(heads(v0_6=OLD, v0_7=SHA)), [f"hevlayer/kit:{V}", "hevlayer/kit:edge"])

    def test_rerun_of_an_older_build_leaves_edge_alone(self):
        # Build N's image job is re-run after N+1 landed: the tip is N+1.
        move, why = eb.moves_edge(SHA, "v0.7", eb.parse_heads(heads(v0_7=OLD)))
        self.assertFalse(move)
        self.assertIn("no longer the tip", why)
        self.assertEqual(self.tags(heads(v0_7=OLD)), [f"hevlayer/kit:{V}"])

    def test_older_train_leaves_edge_alone(self):
        move, why = eb.moves_edge(SHA, "v0.7", eb.parse_heads(heads(v0_7=SHA, v0_8=OLD)))
        self.assertFalse(move)
        self.assertIn("v0.8", why)
        # Numeric, not string, order: v0.10 is newer than v0.9.
        self.assertFalse(eb.moves_edge(SHA, "v0.9", eb.parse_heads(heads(v0_9=SHA, v0_10=OLD)))[0])
        self.assertTrue(eb.moves_edge(SHA, "v0.10", eb.parse_heads(heads(v0_9=OLD, v0_10=SHA)))[0])

    def test_missing_train_or_bad_input_leaves_edge_alone_or_refuses(self):
        self.assertFalse(eb.moves_edge(SHA, "v0.7", eb.parse_heads(""))[0])
        self.assertFalse(eb.moves_edge(SHA, "v0.7", eb.parse_heads(heads(v0_8_1=SHA)))[0])
        for commit, train in [(SHA[:7], "v0.7"), (SHA, "main"), (SHA, "v0.7\n")]:
            with self.assertRaises(eb.EdgeError):
                eb.moves_edge(commit, train, {"v0.7": SHA})

    def test_cli_asks_the_remote(self):
        with tempfile.TemporaryDirectory(prefix="edge-remote-") as tmp:
            remote, work = Path(tmp) / "remote.git", Path(tmp) / "work"
            run = lambda *a: subprocess.check_output(["git", *a], env=ENV, text=True).strip()
            run("init", "-q", "--bare", str(remote))
            run("init", "-q", "-b", "v0.7", str(work))
            run("-C", str(work), "commit", "-q", "--allow-empty", "-m", "n")
            run("-C", str(work), "remote", "add", "origin", str(remote))
            run("-C", str(work), "push", "-q", "origin", "v0.7")
            n = run("-C", str(work), "rev-parse", "HEAD")
            vn = f"0.7.0-edge.{n[:7]}"

            def cli(version, commit):
                out, err = io.StringIO(), io.StringIO()
                with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                    rc = eb.main(["image-tags", "--version", version, "--commit", commit, "--train", "v0.7",
                                  "--repo-dir", str(work)])
                self.assertEqual(rc, 0, err.getvalue())
                return out.getvalue().split()

            self.assertEqual(cli(vn, n), [f"hevlayer/kit:{vn}", "hevlayer/kit:edge"])
            # N+1 lands; re-running N's image job pushes N's own tag only.
            run("-C", str(work), "commit", "-q", "--allow-empty", "-m", "n+1")
            run("-C", str(work), "push", "-q", "origin", "v0.7")
            self.assertEqual(cli(vn, n), [f"hevlayer/kit:{vn}"])
            n1 = run("-C", str(work), "rev-parse", "HEAD")
            self.assertEqual(cli(f"0.7.0-edge.{n1[:7]}", n1)[1:], ["hevlayer/kit:edge"])
            # A newer train opens: v0.7's tip no longer moves :edge.
            run("-C", str(work), "push", "-q", "origin", "HEAD:refs/heads/v0.8")
            self.assertEqual(len(cli(f"0.7.0-edge.{n1[:7]}", n1)), 1)


class StrictPatternTest(unittest.TestCase):
    def test_versions_and_trains_are_ascii_and_whole(self):
        self.assertTrue(eb.EDGE_VERSION.fullmatch(V))
        for bad in [V + "\n", "0.\u0667.0-edge.0123456"]:
            self.assertIsNone(eb.EDGE_VERSION.fullmatch(bad))
        for bad in ["v0.7\n", "v0.\u0667"]:
            self.assertIsNone(eb.TRAIN.fullmatch(bad))
        with self.assertRaises(eb.EdgeError):
            eb.manifest(Path("."), V + "\n", "v0.7", SHA, 1)
        with self.assertRaises(eb.EdgeError):
            eb.plan_prune([], "v0.7\n", 10)


class EdgeWorkflowTest(unittest.TestCase):
    """edge.yml pushes only the tags image-tags prints."""

    def setUp(self):
        self.text = (HERE.parent / ".github" / "workflows" / "edge.yml").read_text()
        # Everything that runs (comments dropped), and the image job alone.
        self.code = "\n".join(l for l in self.text.splitlines() if not l.lstrip().startswith("#"))
        m = re.search(r"^  image:\n(.*?)(?=^  \S|\Z)", self.code, re.M | re.S)
        self.assertIsNotNone(m, "edge.yml has no image job")
        self.image = m.group(1)

    def test_image_job_pushes_only_image_tags_output(self):
        self.assertIn("needs: edge", self.image)
        self.assertIn('edge-build.py image-tags --version "$VERSION" --commit "$GITHUB_SHA" --train "$GITHUB_REF_NAME"',
                      self.image)
        self.assertEqual(self.code.count("edge-build.py image-tags"), 1)
        self.assertIn("--platform linux/amd64,linux/arm64", self.image)
        self.assertIn('--build-arg VERSION="$VERSION"', self.image)
        self.assertIn("--push", self.image)
        # The only tag flag is -t "$t", built from image-tags' output; quoted or
        # spelled --tag, any other is caught.
        self.assertEqual(re.findall(r"""["']?(?:-t|--tag)\b["']?[ =]["']?[^\s);]*""", self.image), ['-t "$t"'])
        self.assertNotIn("--tag", self.code)
        self.assertEqual(self.code.count("docker buildx build"), 1)
        self.assertEqual(self.code.count("--push"), 1)

    def test_never_names_latest_or_a_bare_version(self):
        self.assertNotRegex(self.code, r"kit:latest|:latest\b")
        # Any image the workflow names literally is the edge version itself,
        # and the repo is never named on its own (as in R=hevlayer/kit; "$R:...").
        self.assertEqual(set(re.findall(r"hevlayer/kit:([\w.${}-]+)", self.code)), {"$VERSION"})
        self.assertEqual(self.code.count("hevlayer/kit"), self.code.count("hevlayer/kit:$VERSION"))
        self.assertNotIn("docker tag", self.code)
        self.assertNotIn("docker push", self.code)
        self.assertNotIn("imagetools create", self.code)
        self.assertNotRegex(self.code, r"\$\{\w+%")  # no tags cut down from $VERSION

    def test_check_compares_edge_with_the_pushed_image(self):
        # :edge is checked against the pushed version's digest whenever this run moved it.
        self.assertIn("edge_tag=", self.image)
        self.assertIn("EDGE_TAG: ${{ steps.push.outputs.edge_tag }}", self.image)
        self.assertIn('if [ -n "$EDGE_TAG" ]; then', self.image)
        self.assertIn('retry digest "$EDGE_TAG"', self.image)
        self.assertIn('if [ "$got" != "$want" ]', self.image)
        self.assertIn("for platform in linux/amd64 linux/arm64", self.image)

    def test_publish_does_not_wait_on_the_image(self):
        edge = re.search(r"^  edge:\n(.*?)(?=^  \S)", self.code, re.M | re.S).group(1)
        self.assertNotIn("docker", edge)
        self.assertIn("version: ${{ steps.version.outputs.version }}", edge)


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
