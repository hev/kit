#!/usr/bin/env python3
"""Release preflight (before Adam publishes) and verify (after the train ran).

  scripts/release-check.py preflight 0.7.0           # from a checkout of the draft's source
  scripts/release-check.py preflight 0.7.0 --draft   # also check the draft on GitHub (needs gh)
  scripts/release-check.py verify 0.7.0              # after publish: assets, image, tap, install

Both print one pass/fail table (and append it to $GITHUB_STEP_SUMMARY when
set) and exit 1 if any row failed. Neither writes anything to GitHub, Docker
Hub or the tap: every remote call is a read. The process is
docs/runbooks/release-train.md.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import re
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
REPO = "hev/kit"
IMAGE = "hevlayer/kit"
TAP = "hev/homebrew-tap"
ARCHIVES = ["darwin_arm64.tar.gz", "darwin_amd64.tar.gz", "linux_arm64.tar.gz",
            "linux_amd64.tar.gz", "windows_amd64.zip"]
# Docs pages that state the current release (install lines, download links).
# Every X.Y.Z on these pages must be the release being cut. History belongs in
# the changelog, which is checked separately.
DOCS_PIN_FILES = ["install.mdx", "index.mdx", "windows.mdx", "invited.mdx", "join-a-host.mdx"]
SITE_DOCS = ROOT / "site/src/content/docs"
PLANNED = re.compile(r"^#{1,6}\s.*\bplanned\b", re.I | re.M)
# The train docs branch marks docs for unbuilt features "Coming in X.Y"; main
# describes only what main does, so none may be left at the cut.
COMING = re.compile(r"Coming in \d+\.\d+")
TRAIN_DOCS_DECL = re.compile(r"^<!--\s*train-docs:\s*(\S+)\s*-->\s*$", re.M)
VERSIONISH = re.compile(r"(?<![\d.])v?(\d+\.\d+\.\d+)(?!\.?\d)")


class Table:
    def __init__(self, title: str):
        self.title, self.rows = title, []

    def add(self, area: str, check: str, status: str, detail: str = ""):
        self.rows.append((area, check, status, detail.strip().replace("\n", " ")[:300]))
        print(f"{status:5} {area}: {check}" + (f" — {detail.strip().splitlines()[0][:200]}" if detail.strip() else ""),
              flush=True)

    def failed(self) -> bool:
        return any(r[2] == "FAIL" for r in self.rows)

    def markdown(self) -> str:
        lines = [f"### {self.title}", "", "| Area | Check | Result | Detail |", "|---|---|---|---|"]
        for area, check, status, detail in self.rows:
            lines.append(f"| {area} | {check} | {status} | {detail.replace('|', '/')} |")
        verdict = "FAILED" if self.failed() else "passed"
        return "\n".join(lines) + f"\n\n**{self.title} {verdict}.**\n"


def run(cmd, cwd=ROOT, env=None, timeout=1800):
    p = subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, text=True, timeout=timeout)
    return p.returncode, (p.stdout + p.stderr).strip()


def check_cmd(t: Table, area: str, name: str, cmd, env=None):
    rc, out = run(cmd, env=env)
    t.add(area, name, "PASS" if rc == 0 else "FAIL", "" if rc == 0 else out[-300:])
    return rc == 0


def http_json(url: str):
    req = urllib.request.Request(url, headers={"User-Agent": "kit-release-check"})
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return json.load(r)
    except json.JSONDecodeError:
        return None  # a site's HTML fallback page, not the JSON asked for
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None
        raise


def http_text(url: str) -> str | None:
    req = urllib.request.Request(url, headers={"User-Agent": "kit-release-check"})
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.read().decode()
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None
        raise


def gh_json(*args):
    rc, out = run(["gh", *args])
    if rc != 0:
        raise RuntimeError(out)
    return json.loads(out)


# --- pure checks (tested in scripts/test-release-check.py) ------------------


def notes_problems(text: str | None, version: str) -> list[str]:
    if text is None:
        return [f"docs/releases/v{version}.md does not exist"]
    problems = []
    if PLANNED.search(text):
        problems.append("still has a 'Planned' section; delete it (or move shipped items up) before publishing")
    if "not written yet" in text.lower():
        problems.append("is a placeholder")
    if not text.strip():
        problems.append("is empty")
    return problems


def docs_pin_problems(docs_dir: Path, version: str) -> list[str]:
    problems = []
    for name in DOCS_PIN_FILES:
        p = docs_dir / name
        if not p.exists():
            continue
        for n, line in enumerate(p.read_text().splitlines(), 1):
            for v in VERSIONISH.findall(line):
                if v != version:
                    problems.append(f"{name}:{n} names {v}")
    return problems


def changelog_problem(docs_dir: Path, version: str) -> str | None:
    p = docs_dir / "changelog.mdx"
    if not p.exists():
        return "changelog.mdx is missing"
    if not re.search(rf"^##\s+{re.escape(version)}\s*$", p.read_text(), re.M):
        return f"changelog.mdx has no '## {version}' section"
    return None


def coming_problems(docs_dir: Path) -> list[str]:
    problems = []
    for p in sorted(docs_dir.rglob("*.md*")):
        for n, line in enumerate(p.read_text().splitlines(), 1):
            if COMING.search(line):
                problems.append(f"{p.relative_to(docs_dir)}:{n}")
    return problems


def train_docs_branch(notes: str | None, version: str) -> str:
    """Same rule as release-draft.py: the notes file's train-docs line, else train/vX.Y-docs."""
    m = TRAIN_DOCS_DECL.search(notes or "")
    if m:
        return m.group(1)
    major, minor, _ = version.split(".")
    return f"train/v{major}.{minor}-docs"


def marker(body: str) -> dict | None:
    m = re.search(r"<!-- kit-release-draft version=(\S+) base=(\S+) source=([0-9a-f]{40}) -->", body or "")
    return {"version": m.group(1), "base": m.group(2), "source": m.group(3)} if m else None


# --- preflight ---------------------------------------------------------------


def preflight(version: str, args) -> Table:
    t = Table(f"Release preflight v{version}")
    tag = f"v{version}"
    head = run(["git", "rev-parse", "HEAD"])[1]

    rc, out = run(["git", "ls-remote", "--tags", "origin", f"refs/tags/{tag}"])
    if rc != 0:
        t.add("source", f"tag {tag} not yet created", "FAIL", out)
    else:
        t.add("source", f"tag {tag} not yet created", "FAIL" if out else "PASS",
              "the tag exists: this version is already released" if out else "")

    notes_path = ROOT / f"docs/releases/{tag}.md"
    notes_text = notes_path.read_text() if notes_path.exists() else None
    problems = notes_problems(notes_text, version)
    t.add("notes", f"docs/releases/{tag}.md is final", "FAIL" if problems else "PASS", "; ".join(problems))

    if args.skip_go:
        t.add("go", "vet, windows vet, test", "SKIP", "--skip-go (CI ran them on this commit)")
    else:
        check_cmd(t, "go", "go vet ./...", ["go", "vet", "./..."])
        check_cmd(t, "go", "GOOS=windows go vet ./...", ["go", "vet", "./..."], env={**os.environ, "GOOS": "windows"})
        check_cmd(t, "go", "go test ./...", ["go", "test", "-count=1", "./..."])

    with tempfile.TemporaryDirectory(prefix="kit-preflight-") as tmp:
        dist = Path(tmp) / "dist"
        env = {**os.environ, "GITHUB_REF_NAME": tag, "DIST_DIR": str(dist)}
        if check_cmd(t, "build", "release archives build (all five targets)", ["./.github/build-release.sh"], env=env):
            missing = [a for a in ARCHIVES if not (dist / f"kit_{version}_{a}").exists()]
            t.add("build", "every archive and checksums.txt present",
                  "FAIL" if missing or not (dist / "checksums.txt").exists() else "PASS", ", ".join(missing))
            binary = extract_host_binary(dist, version, Path(tmp))
            if binary is None:
                t.add("build", "candidate reports its version", "SKIP", f"no archive for {platform.system()}/{platform.machine()}")
            else:
                rc, out = run([str(binary), "version"])
                t.add("build", f"candidate `hev version` prints {version}", "PASS" if rc == 0 and out == version else "FAIL", out)
                skills_check(t, binary, Path(tmp))
        check_cmd(t, "image", f"binary pins {IMAGE}:{version} (internal/version test)",
                  ["go", "test", "-count=1", "-run", "TestKitImage", "./internal/version"])
        tags = http_json(f"https://hub.docker.com/v2/repositories/{IMAGE}/tags/{version}")
        t.add("image", f"{IMAGE}:{version} not yet pushed", "FAIL" if tags else "PASS",
              "already on Docker Hub; the train would overwrite it" if tags else "")
        if args.docker:
            ok = check_cmd(t, "image", "dashboard image builds with this version",
                           ["docker", "build", "-q", "--build-arg", f"VERSION={version}", "-t", f"kit-preflight:{version}", "."])
            if ok:
                rc, out = run(["docker", "run", "--rm", f"kit-preflight:{version}", "version"])
                t.add("image", f"image's hev reports {version}", "PASS" if out == version else "FAIL", out)

    if SITE_DOCS.exists():
        p = changelog_problem(SITE_DOCS, version)
        t.add("docs", f"changelog has {version}", "FAIL" if p else "PASS", p or "")
        pins = docs_pin_problems(SITE_DOCS, version)
        t.add("docs", f"install and download pages pin {version}", "FAIL" if pins else "PASS", "; ".join(pins))
        coming = coming_problems(SITE_DOCS)
        t.add("docs", "no 'Coming in X.Y' marker on the release's docs", "FAIL" if coming else "PASS",
              "; ".join(coming[:10]) + (" ..." if len(coming) > 10 else ""))
    else:
        t.add("docs", "changelog and docs pin", "SKIP", "site/ is not on this branch yet; check the site branch by hand")

    branch = train_docs_branch(notes_text, version)
    rc, out = run(["git", "ls-remote", "--heads", "origin", f"refs/heads/{branch}"])
    if rc != 0:
        t.add("docs", f"train docs branch {branch} closed", "FAIL", out)
    else:
        t.add("docs", f"train docs branch {branch} closed", "FAIL" if out else "PASS",
              "still on origin: move its remaining pages to the next train's branch, close its PR "
              "and delete the branch; the preview then follows main" if out else "")

    if args.draft:
        draft_check(t, version, head)
    t.add("manual", "browser validation pass (hev validate NAME, then hev dispatch show NAME)", "TODO",
          "not automated; FOH runs it and records the verdict on the draft PR")
    t.add("manual", "bundled skills reviewed for public content and shipped commands", "TODO", "FOH reads skills/*/SKILL.md diff since the last tag")
    return t


def extract_host_binary(dist: Path, version: str, tmp: Path) -> Path | None:
    osname = {"Linux": "linux", "Darwin": "darwin"}.get(platform.system())
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower())
    archive = dist / f"kit_{version}_{osname}_{arch}.tar.gz"
    if not osname or not arch or not archive.exists():
        return None
    out = tmp / "bin"
    out.mkdir(exist_ok=True)
    with tarfile.open(archive) as tf:
        tf.extract("hev", out, filter="data")
    return out / "hev"


def skills_check(t: Table, binary: Path, tmp: Path):
    home = tmp / "home"
    (home / ".claude").mkdir(parents=True)
    (home / ".codex").mkdir(parents=True)
    env = {"PATH": os.environ.get("PATH", ""), "HOME": str(home), "USERPROFILE": str(home),
           "HEV_CONFIG": str(home / "config.toml")}
    rc, out = run([str(binary), "skills", "setup"], env=env)
    if rc != 0:
        t.add("skills", "hev skills setup in a temporary HOME", "FAIL", out[-300:])
        return
    rc, out = run([str(binary), "skills", "check"], env=env)
    t.add("skills", "hev skills check: every bundled skill current", "PASS" if rc == 0 else "FAIL",
          "" if rc == 0 else out[-300:])


def draft_check(t: Table, version: str, head: str):
    tag = f"v{version}"
    try:
        rels = [json.loads(line) for line in
                run(["gh", "api", "--paginate", "--jq", ".[]", f"repos/{REPO}/releases?per_page=100"])[1].splitlines()
                if line.startswith("{")]
    except Exception as e:  # noqa: BLE001 - report, don't crash the table
        t.add("draft", f"draft {tag} on {REPO}", "FAIL", str(e))
        return
    drafts = [r for r in rels if r.get("tag_name") == tag]
    if len(drafts) != 1:
        t.add("draft", f"exactly one release {tag} on {REPO}", "FAIL", f"found {len(drafts)}")
        return
    d = drafts[0]
    problems = []
    if not d.get("draft"):
        problems.append("already published")
    m = marker(d.get("body", ""))
    if not m:
        problems.append("body has no kit-release-draft marker")
    else:
        if m["version"] != version:
            problems.append(f"marker names {m['version']}")
        if m["source"] != d.get("target_commitish"):
            problems.append("marker source differs from the target commit")
        if m["source"] != head:
            problems.append(f"draft is built from {m['source'][:7]}, this checkout is {head[:7]}")
    if "not written yet" in (d.get("body") or ""):
        problems.append("notes not written")
    t.add("draft", f"draft {tag}: tagged, targets this commit, notes rendered", "FAIL" if problems else "PASS",
          "; ".join(problems))


# --- verify ------------------------------------------------------------------


def verify(version: str, args) -> Table:
    t = Table(f"Release verify v{version}")
    tag = f"v{version}"
    try:
        rel = gh_json("api", f"repos/{REPO}/releases/tags/{tag}")
    except RuntimeError as e:
        t.add("release", f"{tag} published", "FAIL", str(e))
        return t
    t.add("release", f"{tag} published, not draft", "FAIL" if rel.get("draft") else "PASS")
    names = {a["name"]: a["browser_download_url"] for a in rel.get("assets", [])}
    want = [f"kit_{version}_{a}" for a in ARCHIVES] + ["checksums.txt"]
    missing = [n for n in want if n not in names]
    t.add("release", "five archives and checksums.txt attached", "FAIL" if missing else "PASS", ", ".join(missing))
    latest = None
    try:
        latest = gh_json("api", f"repos/{REPO}/releases/latest").get("tag_name")
    except RuntimeError:
        pass
    newest = latest == tag
    if not args.not_latest:
        t.add("release", f"{tag} is the latest release", "PASS" if newest else "FAIL", f"latest is {latest}")

    with tempfile.TemporaryDirectory(prefix="kit-verify-") as tmp:
        tmpdir = Path(tmp)
        osname = {"Linux": "linux", "Darwin": "darwin"}.get(platform.system(), "linux")
        arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine().lower(), "amd64")
        archive = f"kit_{version}_{osname}_{arch}.tar.gz"
        if archive in names and "checksums.txt" in names:
            data = urllib.request.urlopen(names[archive], timeout=120).read()
            sums = urllib.request.urlopen(names["checksums.txt"], timeout=60).read().decode()
            want_sum = next((ln.split()[0] for ln in sums.splitlines() if ln.split()[-1].lstrip("./") == archive), "")
            got = hashlib.sha256(data).hexdigest()
            t.add("install", f"{archive} matches checksums.txt", "PASS" if got == want_sum else "FAIL")
            (tmpdir / archive).write_bytes(data)
            (tmpdir / "bin").mkdir()
            with tarfile.open(tmpdir / archive) as tf:
                tf.extract("hev", tmpdir / "bin", filter="data")
            rc, out = run([str(tmpdir / "bin/hev"), "version"])
            t.add("install", f"fresh download reports {version}", "PASS" if out == version else "FAIL", out)
        else:
            t.add("install", "fresh download reports the version", "FAIL", f"{archive} not attached")

    hub = http_json(f"https://hub.docker.com/v2/repositories/{IMAGE}/tags/{version}")
    t.add("image", f"{IMAGE}:{version} on Docker Hub", "PASS" if hub else "FAIL")
    if hub and not args.not_latest:
        lat = http_json(f"https://hub.docker.com/v2/repositories/{IMAGE}/tags/latest") or {}
        t.add("image", f"{IMAGE}:latest is {version}", "PASS" if lat.get("digest") == hub.get("digest") else "FAIL")

    for formula in ("hev", "kit"):
        text = http_text(f"https://raw.githubusercontent.com/{TAP}/main/Formula/{formula}.rb") or ""
        ok = f"/download/{tag}/" in text or f'version "{version}"' in text or f"/tags/{tag}.tar.gz" in text
        if not args.not_latest:
            t.add("tap", f"{TAP} Formula/{formula}.rb serves {version}", "PASS" if ok else "FAIL")

    pin = http_json("https://hevkit.com/docs-release.json")
    if pin is None:
        t.add("docs", "hevkit.com serves this release's docs", "SKIP", "no docs-release.json yet (production deploy not automated)")
    elif not args.not_latest:
        t.add("docs", "hevkit.com serves this release's docs", "PASS" if pin.get("tag") == tag else "FAIL", f"serves {pin.get('tag')}")
    return t


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("preflight")
    p.add_argument("version")
    p.add_argument("--draft", action="store_true", help="also check the draft release on GitHub (read only)")
    p.add_argument("--docker", action="store_true", help="also build the dashboard image locally")
    p.add_argument("--skip-go", action="store_true", help="skip vet and test (CI already ran them)")
    v = sub.add_parser("verify")
    v.add_argument("version")
    v.add_argument("--not-latest", action="store_true", help="a patch on an older line: skip latest checks")
    args = ap.parse_args(argv)
    version = args.version.lstrip("v")
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        ap.error(f"not a release version: {args.version}")
    t = preflight(version, args) if args.cmd == "preflight" else verify(version, args)
    md = t.markdown()
    print()
    print(md)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as f:
            f.write(md + "\n")
    return 1 if t.failed() else 0


if __name__ == "__main__":
    sys.exit(main())
