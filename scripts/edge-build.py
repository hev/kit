#!/usr/bin/env python3
"""Edge builds: version, manifest, publish, prune and image tags, for .github/workflows/edge.yml.

An edge build is the tip of an open train branch vX.Y, built with the same
script as a release (.github/build-release.sh) and published as a GitHub
pre-release tagged edge-X.Y.Z-edge.<sha>. Hosts that follow the edge channel
(`hev update --channel edge`) fetch it from there without credentials.

Edge tags do not start with "v": the release scripts find the last release
with the glob v[0-9]*.[0-9]*.[0-9]*, which would otherwise match an edge tag.
An edge pre-release is never the latest release, never moves the Homebrew
formula or the dashboard image's latest tag, and is never a draft, so the
draft release (scripts/release-draft.py) never sees it. Its image is
hevlayer/kit:X.Y.Z-edge.<sha>, plus the floating hevlayer/kit:edge when the
build is still the tip of the newest train (so :edge never moves back), and never
:latest or a bare :X.Y.Z, which belong to releases (release.yml).

  scripts/edge-build.py version --branch v0.7                 # X.Y.Z-edge.<sha> for HEAD
  scripts/edge-build.py manifest --dist dist --version V --train v0.7 --commit SHA --sequence N
  scripts/edge-build.py publish --repo hev/kit --dist dist --version V --commit SHA   # gh, contents: write
  scripts/edge-build.py prune --repo hev/kit --train v0.7 --keep 10 [--apply]       # gh, contents: write
  scripts/edge-build.py image-tags --version V --commit SHA --train v0.7   # the image tags to push, one per line
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
# Matched with fullmatch and re.ASCII: no trailing newline, no non-ASCII digits.
TRAIN = re.compile(r"v(\d+)\.(\d+)", re.ASCII)
EDGE_VERSION = re.compile(r"(\d+)\.(\d+)\.(\d+)-edge\.([0-9a-f]{7,40})", re.ASCII)
EDGE_TAG = re.compile(r"edge-((\d+)\.(\d+)\.\d+-edge\.[0-9a-f]{7,40})", re.ASCII)
ASSET = re.compile(r"kit_(.+)_(darwin|linux|windows)_(amd64|arm64)\.(tar\.gz|zip)", re.ASCII)
PICKER_ASSET = re.compile(r"hev-pick_.+_(darwin|linux)_(arm64|amd64)\.tar\.gz")
FULL_SHA = re.compile(r"[0-9a-f]{40}", re.ASCII)
IMAGE_REPO = "hevlayer/kit"
EDGE_IMAGE_TAG = "edge"
PLATFORMS = {("darwin", "arm64"), ("darwin", "amd64"), ("linux", "arm64"), ("linux", "amd64"), ("windows", "amd64")}


class EdgeError(Exception):
    pass


def git(repo_dir: str, *args: str) -> str:
    p = subprocess.run(["git", "-C", repo_dir, *args], capture_output=True, text=True)
    if p.returncode != 0:
        raise EdgeError(f"git {' '.join(args)}: {p.stderr.strip()}")
    return p.stdout.strip()


def edge_version(repo_dir: str, branch: str, ref: str = "HEAD") -> str:
    """X.Y.Z-edge.<sha7>: X.Y.Z is the version the train's draft is for."""
    if not TRAIN.fullmatch(branch):
        raise EdgeError(f"{branch} is not a train branch vX.Y")
    p = subprocess.run([sys.executable, str(HERE / "release-draft.py"), "--repo-dir", repo_dir, "--ref", ref,
                        "--branch", branch, "--print-version"], capture_output=True, text=True)
    if p.returncode != 0:
        raise EdgeError(f"release-draft.py --print-version: {p.stderr.strip()}")
    base = p.stdout.strip()
    sha = git(repo_dir, "rev-parse", "--short=7", ref)
    v = f"{base}-edge.{sha}"
    if not EDGE_VERSION.fullmatch(v):
        raise EdgeError(f"computed {v!r}, not X.Y.Z-edge.<sha>")
    return v


def manifest(dist: Path, version: str, train: str, commit: str, sequence: int, built: str | None = None) -> dict:
    """The manifest for the archives in dist, checked against checksums.txt."""
    if not EDGE_VERSION.fullmatch(version):
        raise EdgeError(f"{version} is not an edge version")
    if not TRAIN.fullmatch(train):
        raise EdgeError(f"{train} is not a train branch")
    if not FULL_SHA.fullmatch(commit):
        raise EdgeError("commit must be a full sha")
    sums = {}
    checksums = dist / "checksums.txt"
    if not checksums.exists():
        raise EdgeError("dist has no checksums.txt")
    for line in checksums.read_text().splitlines():
        parts = line.split()
        if len(parts) == 2:
            sums[parts[1].lstrip("*")] = parts[0].lower()
    assets = []
    for path in sorted(dist.iterdir()):
        m = ASSET.fullmatch(path.name)
        if not m:
            continue
        if m.group(1) != version:
            raise EdgeError(f"{path.name} is not built as {version}")
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if sums.get(path.name) != digest:
            raise EdgeError(f"{path.name}: checksums.txt says {sums.get(path.name)}, file is {digest}")
        assets.append({"name": path.name, "os": m.group(2), "arch": m.group(3), "sha256": digest,
                       "size": path.stat().st_size})
    have = {(a["os"], a["arch"]) for a in assets}
    if have != PLATFORMS:
        raise EdgeError(f"dist has builds for {sorted(have)}, want {sorted(PLATFORMS)}")
    return {
        "schema": 1,
        "version": version,
        "channel": "edge",
        "train": train,
        "commit": commit,
        "sequence": sequence,
        "built": built or datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "assets": assets,
    }


def plan_prune(releases: list[dict], train: str, keep: int) -> list[str]:
    """Tags of this train's edge pre-releases beyond the newest `keep`.

    Only published pre-releases with an edge-X.Y.Z-edge.<sha> tag on this
    train are candidates; releases, drafts and other trains are never touched.
    """
    m = TRAIN.fullmatch(train)
    if not m:
        raise EdgeError(f"{train} is not a train branch")
    if keep < 1:
        raise EdgeError("keep at least one edge build")
    line = (m.group(1), m.group(2))
    ours = []
    for r in releases:
        t = EDGE_TAG.fullmatch(r.get("tag_name") or "")
        if not t or r.get("draft") or not r.get("prerelease") or (t.group(2), t.group(3)) != line:
            continue
        ours.append(r)
    ours.sort(key=lambda r: (r.get("published_at") or r.get("created_at") or ""), reverse=True)
    return [r["tag_name"] for r in ours[keep:]]


def parse_heads(ls_remote: str) -> dict[str, str]:
    """{train: sha} from `git ls-remote origin 'refs/heads/v*'`; non-train branches are dropped."""
    heads = {}
    for line in ls_remote.splitlines():
        parts = line.split()
        if len(parts) == 2 and parts[1].startswith("refs/heads/"):
            name = parts[1][len("refs/heads/"):]
            if TRAIN.fullmatch(name) and FULL_SHA.fullmatch(parts[0]):
                heads[name] = parts[0]
    return heads


def moves_edge(commit: str, train: str, heads: dict[str, str]) -> tuple[bool, str]:
    """Whether the build of `commit` on `train` may move the floating :edge tag, and why.

    Only the current tip of the newest train may. A re-run of an older build's
    image job (its commit is no longer the tip), or a build on an older train
    while a newer one is open, pushes its own tag and leaves :edge alone, so
    :edge never moves backwards.
    """
    m = TRAIN.fullmatch(train)
    if not m:
        raise EdgeError(f"{train} is not a train branch")
    if not FULL_SHA.fullmatch(commit):
        raise EdgeError("commit must be a full sha")
    tip = heads.get(train)
    if tip is None:
        return False, f"{train} is not on the remote"
    if tip != commit:
        return False, f"{commit[:12]} is no longer the tip of {train} ({tip[:12]} is)"
    def line(branch: str) -> tuple[int, int]:
        t = TRAIN.fullmatch(branch)
        return int(t.group(1)), int(t.group(2))

    newer = sorted(b for b in heads if line(b) > line(train))
    if newer:
        return False, f"a newer train is open ({', '.join(newer)})"
    return True, f"{commit[:12]} is the tip of {train}, the newest train"


def image_tags(version: str, move_edge: bool = True) -> list[str]:
    """The tags an edge build pushes: its own version, and the floating edge tag if it may move it.

    Never :latest or a bare :X.Y.Z. Those are release.yml's: users of a
    release pull hevlayer/kit:latest or hevlayer/kit:<release version>, so an
    edge image under either would reach them.
    """
    if not EDGE_VERSION.fullmatch(version):
        raise EdgeError(f"{version!r} is not an edge version X.Y.Z-edge.<sha>")
    tags = [f"{IMAGE_REPO}:{version}"]
    if move_edge:
        tags.append(f"{IMAGE_REPO}:{EDGE_IMAGE_TAG}")
    for t in tags:
        tag = t.rsplit(":", 1)[1]
        if tag == "latest" or re.fullmatch(r"v?\d+\.\d+\.\d+", tag, re.ASCII):
            raise EdgeError(f"refusing release tag {t}")
    return tags


def build_image_tags(version: str, commit: str, train: str, ls_remote: str) -> tuple[list[str], str]:
    """image_tags for this build, with :edge only if moves_edge allows it."""
    if not EDGE_VERSION.fullmatch(version):
        raise EdgeError(f"{version!r} is not an edge version X.Y.Z-edge.<sha>")
    if not commit.startswith(EDGE_VERSION.fullmatch(version).group(4)):
        raise EdgeError(f"{version} was not built from {commit}")
    move, why = moves_edge(commit, train, parse_heads(ls_remote))
    return image_tags(version, move), why


def gh(*args: str, stdin: str | None = None) -> str:
    p = subprocess.run(["gh", *args], input=stdin, capture_output=True, text=True)
    if p.returncode != 0:
        raise EdgeError(f"gh {' '.join(args[:3])}: {p.stderr.strip()}")
    return p.stdout


def notes(version: str, train: str, commit: str) -> str:
    return (f"Edge build of `{train}` at {commit[:12]}: the tip of the open release train, built and tested "
            f"like a release. Not a release, and never marked latest.\n\n"
            f"Hosts that follow edge move to it with `hev update --channel edge`; "
            f"`manifest.json` names each archive's sha256. See https://hevkit.com/docs/updating.\n\n"
            f"<!-- kit-edge version={version} train={train} source={commit} -->\n")


def publish(repo: str, dist: Path, version: str, commit: str, train: str) -> None:
    tag = f"edge-{version}"
    files = sorted(str(p) for p in dist.iterdir() if ASSET.fullmatch(p.name) or PICKER_ASSET.fullmatch(p.name) or p.name in ("checksums.txt", "manifest.json"))
    if not any(f.endswith("manifest.json") for f in files):
        raise EdgeError("dist has no manifest.json")
    exists = subprocess.run(["gh", "release", "view", tag, "--repo", repo, "--json", "isPrerelease,isDraft"],
                            capture_output=True, text=True)
    if exists.returncode == 0:
        state = json.loads(exists.stdout)
        if not state.get("isPrerelease") or state.get("isDraft"):
            raise EdgeError(f"{tag} exists and is not a published pre-release; refusing to touch it")
        gh("release", "upload", tag, *files, "--repo", repo, "--clobber")
        return
    gh("release", "create", tag, *files, "--repo", repo, "--target", commit, "--prerelease", "--latest=false",
       "--title", f"kit {version} (edge)", "--notes", notes(version, train, commit))


def list_releases(repo: str) -> list[dict]:
    out = gh("api", "--paginate", "--jq", ".[]", f"repos/{repo}/releases?per_page=100")
    return [json.loads(line) for line in out.splitlines() if line.strip()]


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    v = sub.add_parser("version")
    v.add_argument("--branch", required=True)
    v.add_argument("--ref", default="HEAD")
    v.add_argument("--repo-dir", default=".")
    m = sub.add_parser("manifest")
    m.add_argument("--dist", default="dist")
    m.add_argument("--version", required=True)
    m.add_argument("--train", required=True)
    m.add_argument("--commit", required=True)
    m.add_argument("--sequence", type=int, required=True)
    p = sub.add_parser("publish")
    p.add_argument("--repo", required=True)
    p.add_argument("--dist", default="dist")
    p.add_argument("--version", required=True)
    p.add_argument("--train", required=True)
    p.add_argument("--commit", required=True)
    r = sub.add_parser("prune")
    r.add_argument("--repo", required=True)
    r.add_argument("--train", required=True)
    r.add_argument("--keep", type=int, default=10)
    r.add_argument("--apply", action="store_true", help="delete; without it, print what would go")
    t = sub.add_parser("image-tags")
    t.add_argument("--version", required=True)
    t.add_argument("--commit", required=True, help="the full sha the image is built from")
    t.add_argument("--train", required=True)
    t.add_argument("--remote", default="origin")
    t.add_argument("--repo-dir", default=".")
    a = ap.parse_args(argv)
    try:
        if a.cmd == "version":
            print(edge_version(a.repo_dir, a.branch, a.ref))
        elif a.cmd == "manifest":
            dist = Path(a.dist)
            doc = manifest(dist, a.version, a.train, a.commit, a.sequence)
            (dist / "manifest.json").write_text(json.dumps(doc, indent=2) + "\n")
            print(json.dumps(doc, indent=2))
        elif a.cmd == "publish":
            publish(a.repo, Path(a.dist), a.version, a.commit, a.train)
        elif a.cmd == "prune":
            for tag in plan_prune(list_releases(a.repo), a.train, a.keep):
                print(("deleting " if a.apply else "would delete ") + tag)
                if a.apply:
                    gh("release", "delete", tag, "--repo", a.repo, "--cleanup-tag", "--yes")
        elif a.cmd == "image-tags":
            heads = git(a.repo_dir, "ls-remote", a.remote, "refs/heads/v*")
            tags, why = build_image_tags(a.version, a.commit, a.train, heads)
            print(("moving :edge: " if len(tags) > 1 else "leaving :edge alone: ") + why, file=sys.stderr)
            print("\n".join(tags))
    except EdgeError as e:
        print(f"edge-build: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
