#!/usr/bin/env python3
"""Edge builds: version, manifest, publish and prune, for .github/workflows/edge.yml.

An edge build is the tip of an open train branch vX.Y, built with the same
script as a release (.github/build-release.sh) and published as a GitHub
pre-release tagged edge-X.Y.Z-edge.<sha>. Hosts that follow the edge channel
(`hev update --channel edge`) fetch it from there without credentials.

Edge tags do not start with "v": the release scripts find the last release
with the glob v[0-9]*.[0-9]*.[0-9]*, which would otherwise match an edge tag.
An edge pre-release is never the latest release, never moves the Homebrew
formula or the dashboard image's latest tag, and is never a draft, so the
draft release (scripts/release-draft.py) never sees it.

  scripts/edge-build.py version --branch v0.7                 # X.Y.Z-edge.<sha> for HEAD
  scripts/edge-build.py manifest --dist dist --version V --train v0.7 --commit SHA --sequence N
  scripts/edge-build.py publish --repo hev/kit --dist dist --version V --commit SHA   # gh, contents: write
  scripts/edge-build.py prune --repo hev/kit --train v0.7 --keep 10 [--apply]       # gh, contents: write
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
TRAIN = re.compile(r"^v(\d+)\.(\d+)$")
EDGE_VERSION = re.compile(r"^(\d+)\.(\d+)\.(\d+)-edge\.([0-9a-f]{7,40})$")
EDGE_TAG = re.compile(r"^edge-((\d+)\.(\d+)\.\d+-edge\.[0-9a-f]{7,40})$")
ASSET = re.compile(r"^kit_(.+)_(darwin|linux|windows)_(amd64|arm64)\.(tar\.gz|zip)$")
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
    if not TRAIN.match(branch):
        raise EdgeError(f"{branch} is not a train branch vX.Y")
    p = subprocess.run([sys.executable, str(HERE / "release-draft.py"), "--repo-dir", repo_dir, "--ref", ref,
                        "--branch", branch, "--print-version"], capture_output=True, text=True)
    if p.returncode != 0:
        raise EdgeError(f"release-draft.py --print-version: {p.stderr.strip()}")
    base = p.stdout.strip()
    sha = git(repo_dir, "rev-parse", "--short=7", ref)
    v = f"{base}-edge.{sha}"
    if not EDGE_VERSION.match(v):
        raise EdgeError(f"computed {v!r}, not X.Y.Z-edge.<sha>")
    return v


def manifest(dist: Path, version: str, train: str, commit: str, sequence: int, built: str | None = None) -> dict:
    """The manifest for the archives in dist, checked against checksums.txt."""
    if not EDGE_VERSION.match(version):
        raise EdgeError(f"{version} is not an edge version")
    if not TRAIN.match(train):
        raise EdgeError(f"{train} is not a train branch")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
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
        m = ASSET.match(path.name)
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
    m = TRAIN.match(train)
    if not m:
        raise EdgeError(f"{train} is not a train branch")
    if keep < 1:
        raise EdgeError("keep at least one edge build")
    line = (m.group(1), m.group(2))
    ours = []
    for r in releases:
        t = EDGE_TAG.match(r.get("tag_name") or "")
        if not t or r.get("draft") or not r.get("prerelease") or (t.group(2), t.group(3)) != line:
            continue
        ours.append(r)
    ours.sort(key=lambda r: (r.get("published_at") or r.get("created_at") or ""), reverse=True)
    return [r["tag_name"] for r in ours[keep:]]


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
    files = sorted(str(p) for p in dist.iterdir() if ASSET.match(p.name) or p.name in ("checksums.txt", "manifest.json"))
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
    except EdgeError as e:
        print(f"edge-build: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
