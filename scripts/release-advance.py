#!/usr/bin/env python3
"""After a release ships: fast-forward main to its tag and open the next train.

Kit's train (docs/runbooks/release-train.md): features merge into the train
branch vX.Y; main does not move between releases and is always the latest
published release. Once release.yml has built, shipped and verified vX.Y.Z:

  1. main is fast-forwarded to the tag, if this is the newest release. Never
     forced: when main is not an ancestor of the tag this stops, exit 1, and
     says so. Someone merges main into the train branch through a PR and cuts
     the next patch; main is never rewound or forced.
  2. For X.Y.0, the next train branch vX.(Y+1) is opened at the tag, unless it
     exists already. If it exists (or after a patch), it must contain the
     tag; when it does not, this warns that vX.Y has to be merged into it
     through a PR. That is not done here.

  scripts/release-advance.py 0.7.0            # print what it would do; changes nothing
  scripts/release-advance.py 0.7.0 --apply    # push main (fast-forward only) and open v0.8

--apply pushes with plain `git push` (no --force, no lease games): git and
GitHub refuse a push that is not a fast-forward. Pushing main needs a token
allowed to push to main under its branch protection; see the runbook.
"""
from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

VERSION_RE = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)$")


class Stop(Exception):
    pass


class Repo:
    def __init__(self, cwd: Path, remote: str):
        self.cwd, self.remote = cwd, remote

    def git(self, *args: str, check: bool = True) -> subprocess.CompletedProcess:
        p = subprocess.run(["git", "-C", str(self.cwd), *args], capture_output=True, text=True)
        if check and p.returncode != 0:
            raise Stop(f"git {' '.join(args)}: {p.stderr.strip()}")
        return p

    def sha(self, ref: str) -> str | None:
        p = self.git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}", check=False)
        return p.stdout.strip() or None

    def is_ancestor(self, a: str, b: str) -> bool:
        return self.git("merge-base", "--is-ancestor", a, b, check=False).returncode == 0

    def release_tags(self) -> list[tuple[int, int, int]]:
        out = self.git("tag", "-l", "v[0-9]*.[0-9]*.[0-9]*").stdout.split()
        return sorted(tuple(int(x) for x in m.groups()) for t in out if (m := VERSION_RE.match(t)))


def plan(repo: Repo, version: tuple[int, int, int]) -> tuple[list[list[str]], list[str]]:
    """The pushes to make and the warnings to report. Raises Stop when main cannot fast-forward."""
    tag = "v%d.%d.%d" % version
    r = repo.remote
    tag_sha = repo.sha(f"refs/tags/{tag}")
    if not tag_sha:
        raise Stop(f"tag {tag} not found; it is created by publishing the draft")
    pushes: list[list[str]] = []
    notes: list[str] = []

    tags = repo.release_tags()
    newest = bool(tags) and tags[-1] == version
    main = repo.sha(f"{r}/main")
    if not newest:
        notes.append(f"{tag} is not the newest release ({'v%d.%d.%d' % tags[-1]}); main stays where it is")
    elif main is None:
        raise Stop(f"{r}/main does not resolve")
    elif main == tag_sha:
        notes.append(f"main is already at {tag} ({tag_sha[:7]})")
    elif repo.is_ancestor(main, tag_sha):
        pushes.append(["push", r, f"{tag_sha}:refs/heads/main"])
        notes.append(f"main {main[:7]} fast-forwards to {tag} {tag_sha[:7]}")
    else:
        raise Stop(f"main ({main[:7]}) is not an ancestor of {tag} ({tag_sha[:7]}): it cannot fast-forward, "
                   "and it is never forced. Stop and report; main stays where it is")

    major, minor, patch = version
    nxt = f"v{major}.{minor + 1}"
    nxt_sha = repo.sha(f"{r}/{nxt}")
    if nxt_sha is None:
        if patch == 0 and newest:
            pushes.append(["push", r, f"{tag_sha}:refs/heads/{nxt}"])
            notes.append(f"opens train branch {nxt} at {tag}")
    elif not repo.is_ancestor(tag_sha, nxt_sha):
        notes.append(f"WARNING: train branch {nxt} does not contain {tag}. Merge v{major}.{minor} into {nxt} "
                     f"through a PR (a merge, not a rebase), so main stays an ancestor of {nxt}")
    else:
        notes.append(f"train branch {nxt} already contains {tag}")
    return pushes, notes


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("version", help="the release that shipped, X.Y.Z")
    ap.add_argument("--apply", action="store_true", help="push (fast-forward main, open the next train)")
    ap.add_argument("--remote", default="origin")
    ap.add_argument("--repo-dir", default=".")
    ap.add_argument("--no-fetch", action="store_true", help="use the remote-tracking refs as they are")
    args = ap.parse_args(argv)
    m = VERSION_RE.match(args.version)
    if not m:
        ap.error(f"not a release version: {args.version}")
    version = tuple(int(x) for x in m.groups())
    repo = Repo(Path(args.repo_dir), args.remote)
    try:
        if not args.no_fetch:
            repo.git("fetch", "--quiet", "--prune", "--tags", args.remote)
        pushes, notes = plan(repo, version)  # type: ignore[arg-type]
    except Stop as e:
        print(f"release-advance: STOP: {e}", file=sys.stderr)
        return 1
    for n in notes:
        print(f"release-advance: {n}")
    for p in pushes:
        cmd = "git " + " ".join(p)
        if not args.apply:
            print(f"release-advance: would run: {cmd}")
            continue
        r = repo.git(*p, check=False)
        if r.returncode != 0:
            print(f"release-advance: STOP: {cmd} failed: {r.stderr.strip()}", file=sys.stderr)
            return 1
        print(f"release-advance: ran: {cmd}")
    if pushes and not args.apply:
        print("release-advance: dry run; nothing pushed (pass --apply)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
