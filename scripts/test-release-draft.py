#!/usr/bin/env python3
"""Tests for scripts/release-draft.py. Each test builds a scratch git repo.

  python3 scripts/test-release-draft.py
"""
import importlib.util
import io
import json
import os
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("release_draft", HERE / "release-draft.py")
rd = importlib.util.module_from_spec(spec)
sys.modules["release_draft"] = rd
spec.loader.exec_module(rd)

ENV = {**os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
       "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
       "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
       "GIT_AUTHOR_DATE": "2026-11-01T00:00:00Z", "GIT_COMMITTER_DATE": "2026-11-01T00:00:00Z"}


class Repo:
    def __init__(self, root: Path):
        self.root = root
        self.git("init", "-q", "-b", "main")

    def git(self, *args, env=None):
        return subprocess.check_output(["git", "-C", str(self.root), *args], env=env or ENV, text=True).strip()

    def commit(self, subject, files, body="", author=None):
        for path, text in files.items():
            p = self.root / path
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(text)
        self.git("add", "-A")
        env = dict(ENV)
        if author:
            env["GIT_AUTHOR_EMAIL"] = author
        msg = subject + ("\n\n" + body if body else "")
        self.git("-c", "commit.gpgsign=false", "commit", "-q", "-m", msg, env=env)
        return self.git("rev-parse", "HEAD")


class FakeGH:
    """Records gh calls and serves releases from a list."""

    def __init__(self, releases=None):
        self.releases = releases or []
        self.calls = []

    def __call__(self, args, stdin=None):
        self.calls.append((args, stdin))
        if args[:2] == ["api", "--paginate"]:
            return "".join(json.dumps(r) + "\n" for r in self.releases)
        if args[:3] == ["api", "--method", "POST"]:
            r = {**json.loads(stdin), "id": 100 + len(self.releases)}
            self.releases.append(r)
            return json.dumps(r)
        if args[:3] == ["api", "--method", "PATCH"]:
            rid = int(args[3].rsplit("/", 1)[1])
            r = next(r for r in self.releases if r["id"] == rid)
            r.update(json.loads(stdin))
            return json.dumps(r)
        if args[0] == "api":
            rid = int(args[1].rsplit("/", 1)[1])
            return json.dumps(next(r for r in self.releases if r["id"] == rid))
        raise AssertionError(f"unexpected gh call {args}")

    def writes(self):
        return [a for a, _ in self.calls if "--method" in a]


def no_gh(args, stdin=None):
    raise AssertionError(f"gh called without --apply: {args}")


class ReleaseDraftTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="release-draft-test-")
        self.repo = Repo(Path(self.tmp.name))
        r = self.repo
        r.commit("Initial", {"README.md": "kit\n"})
        r.git("tag", "v0.6.2")
        r.commit("Short invite codes for hev nodes (#183)", {"internal/nodes/x.go": "package nodes\n"})
        r.commit("Bump github.com/spf13/pflag from 1.0.10 to 1.1.0 (#201)", {"go.mod": "module x\n"},
                 author="49699333+dependabot[bot]@users.noreply.github.com")
        r.commit("fix: hev s finds the macOS daemon (#171)", {"internal/daemon/d.go": "package daemon\n"})
        r.commit("Explain dispatch outcomes (#185)", {"docs/dispatch.md": "# Dispatch\n"})
        r.commit("Tidy a test", {"internal/nodes/x_test.go": "package nodes\n"}, body="Release-Kind: skip")
        r.commit("Direct commit without a PR", {"cmd/hev/main.go": "package main\n"})
        self.git = rd.Git(r.root)

    def tearDown(self):
        self.tmp.cleanup()

    def test_dry_run_renders_groups_and_marker(self):
        d = rd.render(self.git)
        self.assertEqual(d.version, "0.7.0")
        self.assertEqual(d.base, "v0.6.2")
        self.assertEqual(d.title, "kit v0.7.0")
        b = d.body
        self.assertIn("Release notes not written yet", b)
        self.assertIn("## Merged since v0.6.2", b)
        changes = b.index("### Changes")
        fixes = b.index("### Fixes")
        docs = b.index("### Docs")
        deps = b.index("### Dependencies")
        self.assertLess(changes, fixes)
        self.assertLess(fixes, docs)
        self.assertLess(docs, deps)
        self.assertIn("- Short invite codes for hev nodes ([#183](https://github.com/hev/kit/pull/183))", b)
        self.assertIn("- fix: hev s finds the macOS daemon ([#171]", b[fixes:docs])
        self.assertIn("Explain dispatch outcomes", b[docs:deps])
        self.assertIn("Bump github.com/spf13/pflag", b[deps:])
        self.assertIn("- Direct commit without a PR ([", b)
        self.assertNotIn("Tidy a test", b)
        self.assertIn("No preview yet", b)
        self.assertNotIn("http://", b.split("## Docs preview")[1])
        self.assertEqual(rd.parse_marker(b), {"version": "0.7.0", "base": "v0.6.2", "source": d.source})
        self.assertTrue(b.endswith(f"source={d.source} -->\n"))

    def test_render_is_deterministic(self):
        self.assertEqual(rd.render(self.git).body, rd.render(self.git).body)

    def test_notes_file_leads_and_declares_version(self):
        self.repo.commit("Notes for 0.7.0 (#190)", {"docs/releases/v0.7.0.md": "## Highlights\n\n- Contexts.\n"})
        d = rd.render(self.git)
        self.assertTrue(d.body.startswith("## Highlights\n\n- Contexts.\n"))
        self.assertNotIn("not written yet", d.body)
        self.assertIn("### Release process", d.body)

        # A notes file for a later minor does not jump the train ahead of 0.7.0
        # unless 0.7.0 is tagged.
        self.repo.commit("Notes for 0.8.0", {"docs/releases/v0.8.0.md": "## Next\n"})
        self.assertEqual(rd.render(self.git).version, "0.7.0")
        self.repo.git("tag", "v0.7.0")
        d = rd.render(self.git)
        self.assertEqual((d.version, d.base), ("0.8.0", "v0.7.0"))
        self.assertTrue(d.body.startswith("## Next"))
        self.assertIn("Nothing yet.", d.body)

    def test_patch_step_and_explicit_version(self):
        self.assertEqual(rd.render(self.git, step="patch").version, "0.6.3")
        self.assertEqual(rd.render(self.git, version="v0.9.1").version, "0.9.1")
        with self.assertRaises(rd.DraftError):
            rd.render(self.git, version="0.6.1")
        with self.assertRaises(rd.DraftError):
            rd.render(self.git, version="0.7")

    def test_docs_preview_links_pages_changed_since_base(self):
        self.repo.commit("Site onto main (#210)", {
            "site/package.json": "{}\n",
            "site/src/content/docs/index.mdx": "---\n---\n",
            "site/src/content/docs/context.mdx": "---\n---\n",
        })
        self.repo.commit("Retire a page (#211)", {"site/src/content/docs/old.mdx": "x\n"})
        self.repo.git("rm", "-q", "site/src/content/docs/old.mdx")
        self.repo.git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "Remove old page (#212)")
        d = rd.render(self.git)
        self.assertIn("docs preview on the mini", d.body)
        self.assertIn("- `/docs/context`", d.body)
        self.assertIn("- `/docs`", d.body)
        self.assertNotIn("http", d.body.split("## Docs preview")[1])
        self.assertNotIn("old", d.body.split("## Docs preview")[1])
        self.assertIn("### Docs", d.body)
        d = rd.render(self.git, preview="http://preview.example:4342/")
        self.assertIn("<http://preview.example:4342/docs>", d.body)
        self.assertIn("- [context](http://preview.example:4342/docs/context)", d.body)
        self.assertIn("- [index](http://preview.example:4342/docs)", d.body)

    def test_train_docs_branch_lists_planned_pages(self):
        r = self.repo
        r.commit("Site onto main (#210)", {
            "site/package.json": "{}\n",
            "site/src/content/docs/index.mdx": "---\n---\n",
        })
        main = r.git("rev-parse", "HEAD")
        # No train docs branch: the preview follows main.
        d = rd.render(self.git)
        self.assertIn("which follows `main`.", d.body)
        self.assertNotIn("planned features", d.body)
        # The conventional branch, train/v0.7-docs, carries docs for an unbuilt feature.
        r.git("checkout", "-q", "-b", "train/v0.7-docs")
        r.commit("docs: 0.7 train docs", {"site/src/content/docs/context.mdx": "Coming in 0.7\n",
                                          "site/src/content/docs/index.mdx": "---\nx\n---\n"})
        r.git("checkout", "-q", "main")
        d = rd.render(self.git, preview="http://mini:4371")
        self.assertEqual(d.source, main)
        self.assertIn("follows the train docs branch `train/v0.7-docs`", d.body)
        self.assertIn('marked "Coming in 0.7"', d.body)
        planned = d.body.split("not on `main` yet:")[1]
        self.assertIn("- [context](http://mini:4371/docs/context)", planned)
        self.assertIn("- [index](http://mini:4371/docs)", planned)
        self.assertEqual(d.body, rd.render(self.git, preview="http://mini:4371").body)
        # Main moves on; pages already merged from the branch's base are not planned.
        r.commit("Context lands with its docs (#220)", {"site/src/content/docs/context.mdx": "Coming in 0.7\n"})
        r.git("checkout", "-q", "train/v0.7-docs")
        r.git("-c", "commit.gpgsign=false", "rebase", "-q", "main")
        r.git("checkout", "-q", "main")
        d = rd.render(self.git)
        planned = d.body.split("not on `main` yet:")[1]
        self.assertNotIn("context", planned)
        self.assertIn("- `/docs`", planned)
        # A notes file can name a different branch (0.7's is a dispatch branch).
        r.git("branch", "-q", "-m", "train/v0.7-docs", "d/kit-docs-07")
        self.assertNotIn("planned features", rd.render(self.git).body)
        r.commit("0.7 notes", {"docs/releases/v0.7.0.md": "# kit 0.7.0\n\n<!-- train-docs: d/kit-docs-07 -->\n"})
        self.assertIn("follows the train docs branch `d/kit-docs-07`", rd.render(self.git).body)
        r.git("branch", "-q", "other", "d/kit-docs-07")
        self.assertIn("train docs branch `other`", rd.render(self.git, docs_branch="other").body)
        r.git("branch", "-q", "-D", "other")
        # At the cut the branch is closed: the preview follows main again.
        r.git("branch", "-q", "-D", "d/kit-docs-07")
        d = rd.render(self.git)
        self.assertIn("which follows `main`.", d.body)
        self.assertNotIn("not on `main` yet", d.body)
        # The published body never names the train docs branch.
        r.git("tag", "v0.7.0")
        self.assertNotIn("train docs", rd.render(self.git, version="0.7.0", published=True).body)

    def test_published_body_at_the_tag(self):
        self.repo.commit("Site onto main (#210)", {"site/package.json": "{}\n",
                                                   "site/src/content/docs/context.mdx": "---\n---\n"})
        self.repo.commit("Notes (#211)", {"docs/releases/v0.7.0.md": "## Highlights\n"})
        draft = rd.render(self.git, preview="http://preview.example:4342")
        self.repo.git("tag", "v0.7.0")
        pub = rd.render(self.git, ref="v0.7.0", version="0.7.0", published=True)
        self.assertEqual((pub.base, pub.source), ("v0.6.2", draft.source))
        self.assertNotIn("preview", pub.body.lower())
        self.assertIn("## Docs\n\n<https://hevkit.com/docs>", pub.body)
        self.assertIn("- [context](https://hevkit.com/docs/context)", pub.body)
        # Everything above the docs section is the draft Adam read.
        self.assertEqual(pub.body.split("## Docs")[0], draft.body.split("## Docs")[0])
        with redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
            self.assertEqual(rd.main(["--repo-dir", str(self.repo.root), "--published", "--apply",
                                      "--version", "0.7.0", "--ref", "v0.7.0"], gh=no_gh), 1)

    def test_merge_commit_pr_title(self):
        self.repo.git("checkout", "-q", "-b", "feature")
        self.repo.commit("work", {"pkg/a.go": "package a\n"})
        self.repo.git("checkout", "-q", "main")
        self.repo.git("-c", "commit.gpgsign=false", "merge", "-q", "--no-ff", "feature",
                      "-m", "Merge pull request #220 from hev/feature\n\nAdd hev context")
        self.assertIn("- Add hev context ([#220]", rd.render(self.git).body)

    def test_main_dry_run_never_calls_gh(self):
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            rc = rd.main(["--repo-dir", str(self.repo.root)], gh=no_gh)
        self.assertEqual(rc, 0)
        self.assertIn("## Merged since v0.6.2", out.getvalue())
        self.assertIn("GitHub not touched", err.getvalue())
        with redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
            self.assertEqual(rd.main(["--repo-dir", str(self.repo.root), "--json"], gh=no_gh), 0)

    def test_upsert_creates_then_is_idempotent_then_updates(self):
        d = rd.render(self.git)
        gh = FakeGH()
        self.assertEqual(rd.upsert(gh, "hev/kit", d), "created")
        rel = gh.releases[0]
        self.assertEqual((rel["tag_name"], rel["target_commitish"], rel["draft"], rel["name"]),
                         ("v0.7.0", d.source, True, "kit v0.7.0"))
        self.assertEqual(rd.upsert(gh, "hev/kit", rd.render(self.git)), "unchanged")
        self.assertEqual(len(gh.writes()), 1)
        self.repo.commit("Another change (#230)", {"pkg/b.go": "package b\n"})
        d2 = rd.render(self.git)
        self.assertEqual(rd.upsert(gh, "hev/kit", d2), "updated")
        self.assertEqual(gh.releases[0]["target_commitish"], d2.source)
        self.assertIn("#230", gh.releases[0]["body"])
        self.assertEqual(len(gh.releases), 1)
        for args in gh.writes():
            self.assertNotIn("publish", " ".join(args))
        for _, stdin in gh.calls:
            if stdin:
                self.assertTrue(json.loads(stdin)["draft"])

    def test_upsert_restores_untagged_draft(self):
        d = rd.render(self.git)
        gh = FakeGH([{"id": 7, "draft": True, "name": "kit v0.7.0", "tag_name": "untagged-abc",
                      "target_commitish": "main", "body": "old"}])
        self.assertEqual(rd.upsert(gh, "hev/kit", d), "updated")
        self.assertEqual(gh.releases[0]["tag_name"], "v0.7.0")

    def test_upsert_refuses_published_and_duplicates(self):
        d = rd.render(self.git)
        gh = FakeGH([{"id": 1, "draft": False, "name": "kit v0.7.0", "tag_name": "v0.7.0",
                      "html_url": "https://github.com/hev/kit/releases/tag/v0.7.0"}])
        with self.assertRaisesRegex(rd.DraftError, "already published"):
            rd.upsert(gh, "hev/kit", d)
        gh = FakeGH([{"id": 1, "draft": True, "name": "kit v0.7.0", "tag_name": "v0.7.0", "body": ""},
                     {"id": 2, "draft": True, "name": "kit v0.7.0", "tag_name": "untagged-x", "body": ""}])
        with self.assertRaisesRegex(rd.DraftError, "more than one draft"):
            rd.upsert(gh, "hev/kit", d)
        self.assertEqual(gh.writes(), [])

    def test_upsert_stops_if_published_between_read_and_write(self):
        d = rd.render(self.git)
        gh = FakeGH([{"id": 3, "draft": True, "name": "kit v0.7.0", "tag_name": "v0.7.0", "body": "old"}])
        real = gh.__call__

        def racing(args, stdin=None):
            if args == ["api", "repos/hev/kit/releases/3"]:
                gh.releases[0]["draft"] = False
            return real(args, stdin)

        with self.assertRaisesRegex(rd.DraftError, "published while rendering"):
            rd.upsert(racing, "hev/kit", d)
        self.assertEqual(gh.writes(), [])


if __name__ == "__main__":
    unittest.main(verbosity=2)
