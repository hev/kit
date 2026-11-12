#!/usr/bin/env python3
"""Render, and with --apply upsert, the draft GitHub release for kit's next version.

The draft is the release train's ledger (docs/runbooks/release-train.md). Each
train has a branch vX.Y; features merge into it, and main does not move until
the release ships. The draft for vX.Y.Z is rendered from the train branch's
tip and targets that commit. Its body is a pure function of the git tree at
one commit:

  1. docs/releases/vX.Y.Z.md at that commit, or a "notes not written yet"
     line when the file does not exist;
  2. one line per pull request merged since the last release tag, grouped by
     kind;
  3. the docs pages changed since that tag, linked into the docs preview when
     --preview-url (or $KIT_DOCS_PREVIEW_URL) names it, and, while the train
     has a train docs branch, the pages on that branch for features not merged
     yet (the preview follows that branch until the cut);
  4. a marker naming the version, the base tag and the source commit.

Nothing in the body depends on the clock or on GitHub, so rendering twice from
the same commit (and the same train docs branch tip) gives the same bytes and --apply on an unchanged draft is a
no-op. Without --apply the script only prints; it never calls gh.

  scripts/release-draft.py --ref origin/v0.7 --branch v0.7           # print the body
  scripts/release-draft.py --ref origin/v0.7 --branch v0.7 --apply   # create or update the draft on hev/kit

--apply never publishes. It refuses to touch a release that is already
published, and refuses to guess between two drafts for one version.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

DEFAULT_REPO = "hev/kit"
NOTES_DIR = "docs/releases"
SITE_DOCS = "site/src/content/docs/"
SITE_MARKER = "site/package.json"
PUBLIC_SITE = "https://hevkit.com"
VERSION_RE = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")
TAG_GLOB = "v[0-9]*.[0-9]*.[0-9]*"
# A notes file names its train docs branch with this line (an HTML comment, so
# it does not show in the release body). Without one the convention applies:
# train/vX.Y-docs.
TRAIN_DOCS_DECL = re.compile(r"^<!--\s*train-docs:\s*(\S+)\s*-->\s*$", re.M)
# A train branch: vX.Y. It carries X.Y.0 and, after it ships, the X.Y.Z patches.
TRAIN_BRANCH = re.compile(r"^v(\d+)\.(\d+)$")

# Groups in the order they appear in the body. A PR goes in the first group
# whose rule matches (see classify).
GROUPS = [
    ("changes", "Changes"),
    ("fixes", "Fixes"),
    ("docs", "Docs"),
    ("release", "Release process"),
    ("dependencies", "Dependencies"),
]
GROUP_KEYS = {key for key, _ in GROUPS} | {"skip"}
KIND_TRAILER = re.compile(r"^Release-Kind:\s*(\S+)\s*$", re.M | re.I)
SQUASH_PR = re.compile(r"\s*\(#(\d+)\)\s*$")
MERGE_PR = re.compile(r"^Merge pull request #(\d+) from \S+")
CONVENTIONAL = re.compile(r"^(\w+)(\([^)]*\))?!?:\s*")


class DraftError(Exception):
    pass


def parse_version(text: str) -> tuple[int, int, int]:
    m = VERSION_RE.match(text.lstrip("v"))
    if not m:
        raise DraftError(f"not a release version: {text!r} (want X.Y.Z)")
    return tuple(int(p) for p in m.groups())  # type: ignore[return-value]


def fmt(v: tuple[int, int, int]) -> str:
    return "%d.%d.%d" % v


def train_line(branch: str | None) -> tuple[int, int] | None:
    """(X, Y) for a train branch vX.Y; None for anything else."""
    m = TRAIN_BRANCH.match(branch or "")
    return (int(m.group(1)), int(m.group(2))) if m else None


# --- git -----------------------------------------------------------------


class Git:
    def __init__(self, cwd: Path):
        self.cwd = cwd

    def run(self, *args: str, check: bool = True) -> str:
        p = subprocess.run(["git", "-C", str(self.cwd), *args], capture_output=True, text=True)
        if check and p.returncode != 0:
            raise DraftError(f"git {' '.join(args)}: {p.stderr.strip()}")
        return p.stdout if p.returncode == 0 else ""

    def sha(self, ref: str) -> str:
        return self.run("rev-parse", "--verify", f"{ref}^{{commit}}").strip()

    def tags(self) -> set[str]:
        return {t for t in self.run("tag", "-l", TAG_GLOB).split() if VERSION_RE.match(t[1:])}

    def base_tag(self, ref: str) -> str | None:
        out = self.run("describe", "--tags", "--abbrev=0", "--match", TAG_GLOB, ref, check=False).strip()
        return out or None

    def show(self, ref: str, path: str) -> str | None:
        p = subprocess.run(["git", "-C", str(self.cwd), "show", f"{ref}:{path}"], capture_output=True)
        return p.stdout.decode("utf-8") if p.returncode == 0 else None

    def exists(self, ref: str, path: str) -> bool:
        return subprocess.run(["git", "-C", str(self.cwd), "cat-file", "-e", f"{ref}:{path}"],
                              capture_output=True).returncode == 0

    def ls(self, ref: str, path: str) -> list[str]:
        return self.run("ls-tree", "--name-only", ref, path.rstrip("/") + "/", check=False).split()


# --- version ---------------------------------------------------------------


def resolve_version(git: Git, ref: str, base: str | None, explicit: str | None,
                    line: tuple[int, int] | None = None) -> str:
    """The version this draft is for.

    On a train branch vX.Y (line) the version is always X.Y.Z: explicit must
    agree; otherwise the lowest untagged docs/releases/vX.Y.Z.md above the
    base tag (a notes file is how a release is declared), else X.Y.0 for a new
    train, else the next patch after the base tag when the base is on the
    line already (X.Y.0 has shipped). Off a train branch (a local run), the
    lowest declared notes file, else the next minor.
    """
    if explicit:
        v = parse_version(explicit)
        if line and v[:2] != line:
            raise DraftError(f"version {fmt(v)} is not on train branch v{line[0]}.{line[1]}")
        return fmt(v)
    base_v = parse_version(base) if base else (0, 0, 0)
    tags = git.tags()
    declared = []
    for name in git.ls(ref, NOTES_DIR):
        m = re.match(r"^v(\d+\.\d+\.\d+)\.md$", Path(name).name)
        if not m:
            continue
        v = parse_version(m.group(1))
        if v > base_v and f"v{fmt(v)}" not in tags and (line is None or v[:2] == line):
            declared.append(v)
    if declared:
        return fmt(min(declared))
    if line:
        if base_v[:2] == line:
            return fmt((line[0], line[1], base_v[2] + 1))
        return fmt((line[0], line[1], 0))
    return fmt((base_v[0], base_v[1] + 1, 0))


# --- merge log ------------------------------------------------------------


@dataclass
class Entry:
    sha: str
    title: str
    pr: int | None
    kind: str
    files: list[str] = field(default_factory=list)


def classify(subject: str, body: str, author: str, files: list[str]) -> str:
    m = KIND_TRAILER.search(body)
    if m and m.group(1).lower() in GROUP_KEYS:
        return m.group(1).lower()
    if "dependabot" in author.lower() or subject.startswith("Bump "):
        return "dependencies"
    conv = CONVENTIONAL.match(subject)
    prefix = conv.group(1).lower() if conv else ""
    if prefix == "fix" or re.match(r"^(fix|fixes|fixed)\b", subject, re.I):
        return "fixes"
    if files and all(f.startswith(f"{NOTES_DIR}/") for f in files):
        return "release"
    if prefix == "docs" or (files and all(f.startswith(("docs/", "site/")) for f in files)):
        return "docs"
    return "changes"


def merge_log(git: Git, base: str | None, ref: str) -> list[Entry]:
    rng = f"{base}..{ref}" if base else ref
    raw = git.run("log", "--first-parent", "--reverse", "--format=%H%x1f%s%x1f%ae%x1f%b%x1e", rng)
    entries = []
    for rec in raw.split("\x1e"):
        rec = rec.strip("\n")
        if not rec:
            continue
        sha, subject, author, body = rec.split("\x1f", 3)
        title, pr = subject, None
        m = SQUASH_PR.search(subject)
        if m:
            pr, title = int(m.group(1)), SQUASH_PR.sub("", subject)
        else:
            m = MERGE_PR.match(subject)
            if m:
                pr = int(m.group(1))
                title = body.strip().splitlines()[0] if body.strip() else subject
        # Against the first parent, so a merge commit lists what the PR brought in.
        if git.run("rev-parse", "--verify", "-q", f"{sha}^1", check=False).strip():
            files = git.run("diff", "--name-only", f"{sha}^1", sha).split()
        else:
            files = git.run("show", "--name-only", "--format=", sha).split()
        kind = classify(title, body, author, files)
        if kind != "skip":
            entries.append(Entry(sha, title.strip(), pr, kind, files))
    return entries


def changed_doc_pages(git: Git, base: str | None, ref: str) -> list[str]:
    if not git.exists(ref, SITE_MARKER):
        return []
    if base:
        names = git.run("diff", "--name-only", "--diff-filter=AMR", base, ref, "--", SITE_DOCS).split()
    else:
        names = git.run("ls-tree", "-r", "--name-only", ref, SITE_DOCS).split()
    slugs = set()
    for name in names:
        m = re.match(re.escape(SITE_DOCS) + r"(.+)\.mdx?$", name)
        if m and git.exists(ref, name):
            slugs.add(m.group(1))
    return sorted(slugs)


def train_docs_branch(notes: str | None, version: str) -> str:
    """The train docs branch for a version: declared in its notes file, else train/vX.Y-docs."""
    m = TRAIN_DOCS_DECL.search(notes or "")
    if m:
        return m.group(1)
    major, minor, _ = parse_version(version)
    return f"train/v{major}.{minor}-docs"


def resolve_docs_ref(git: Git, branch: str) -> str | None:
    """origin/BRANCH, else BRANCH; None when neither exists (no train docs branch, or it closed at the cut)."""
    for ref in (f"origin/{branch}", branch):
        out = git.run("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}", check=False).strip()
        if out:
            return ref
    return None


def planned_doc_pages(git: Git, source: str, docs_ref: str) -> list[str]:
    """Pages the train docs branch adds or changes relative to its merge base with the source."""
    names = git.run("diff", "--name-only", "--diff-filter=AMR", f"{source}...{docs_ref}", "--", SITE_DOCS).split()
    slugs = set()
    for name in names:
        m = re.match(re.escape(SITE_DOCS) + r"(.+)\.mdx?$", name)
        if m:
            slugs.add(m.group(1))
    return sorted(slugs)


def doc_path(slug: str) -> str:
    return "/docs" if slug == "index" else f"/docs/{slug}"


# --- render ---------------------------------------------------------------


@dataclass
class Draft:
    version: str
    base: str | None
    source: str
    title: str
    body: str
    entries: int = 0      # merged since base
    declared: bool = False  # docs/releases/vX.Y.Z.md exists at source

    @property
    def tag(self) -> str:
        return f"v{self.version}"


def render(git: Git, ref: str = "HEAD", version: str | None = None, base: str | None = None,
           branch: str | None = None, repo: str = DEFAULT_REPO, preview: str | None = None,
           published: bool = False, docs_branch: str | None = None) -> Draft:
    """Render the body for the release on train branch `branch` (vX.Y) from commit `ref`."""
    source = git.sha(ref)
    if base is None:
        base = git.base_tag(source)
        if version and base == f"v{fmt(parse_version(version))}":
            # Rendering at the release's own tag (release.yml after publish).
            base = git.base_tag(f"{source}^")
    version = resolve_version(git, source, base, version, train_line(branch))
    if base and parse_version(base) >= parse_version(version):
        raise DraftError(f"version {version} is not after base {base}")
    preview = (preview or "").rstrip("/")
    tag = f"v{version}"
    since = base or "the first commit"
    out: list[str] = []

    notes = git.show(source, f"{NOTES_DIR}/{tag}.md")
    if notes and notes.strip():
        out.append(notes.strip())
    else:
        out.append(f"> Release notes not written yet. Add `{NOTES_DIR}/{tag}.md` to "
                   f"{'`' + branch + '`' if branch else 'the train branch'}; "
                   "the next run of the draft workflow puts it here.")

    entries = merge_log(git, base, source)
    out.append("")
    out.append(f"## Merged since {since}")
    out.append("")
    if not entries:
        out.append("Nothing yet.")
    for key, label in GROUPS:
        group = [e for e in entries if e.kind == key]
        if not group:
            continue
        out.append(f"### {label}")
        out.append("")
        for e in group:
            if e.pr is not None:
                out.append(f"- {e.title} ([#{e.pr}](https://github.com/{repo}/pull/{e.pr}))")
            else:
                out.append(f"- {e.title} ([{e.sha[:7]}](https://github.com/{repo}/commit/{e.sha}))")
        out.append("")
    if out[-1] == "":
        out.pop()

    out.append("")
    if published:
        # The public body: the docs are on hevkit.com now, which the site
        # workflow pins to this release; the private preview is not named.
        out.append("## Docs")
        out.append("")
        out.append(f"<{PUBLIC_SITE}/docs>")
        pages = changed_doc_pages(git, base, source) if git.exists(source, SITE_MARKER) else []
        if pages:
            out.append("")
            out.append(f"Pages changed since {since}:")
            out.append("")
            for slug in pages:
                out.append(f"- [{slug}]({PUBLIC_SITE}{doc_path(slug)})")
    else:
        out.extend(preview_section(git, base, source, since, preview, version,
                                   docs_branch or train_docs_branch(notes, version), branch))

    out.append("")
    out.append(f"<!-- Generated by scripts/release-draft.py. Edit {NOTES_DIR}/{tag}.md, not this body. -->")
    out.append(f"<!-- kit-release-draft version={version} base={base or '-'} source={source} -->")
    return Draft(version, base, source, f"kit {tag}", "\n".join(out) + "\n",
                 entries=len(entries), declared=bool(notes and notes.strip()))


def preview_section(git: Git, base: str | None, source: str, since: str, preview: str,
                    version: str, docs_branch: str, branch: str | None) -> list[str]:
    out = ["## Docs preview", ""]
    if not git.exists(source, SITE_MARKER):
        out.append("No preview yet: the docs site is not on this branch.")
        return out
    # The preview is served on the private network, so its address comes from
    # the caller and this section is dropped when the release is published
    # (release.yml); the public notes link hevkit.com instead. While the train
    # has a train docs branch the preview follows it: it is the train branch
    # plus the docs for planned features, so every page below resolves there,
    # shipped or not.
    on = f"`{branch}`" if branch else "the train branch"
    docs_ref = resolve_docs_ref(git, docs_branch)
    planned = planned_doc_pages(git, source, docs_ref) if docs_ref else []
    major, minor, _ = parse_version(version)
    follows = (f"the train docs branch `{docs_branch}` ({on} plus the docs for features not merged yet, "
               f"marked \"Coming in {major}.{minor}\")") if docs_ref else on
    if preview:
        out.append(f"This release's docs: <{preview}/docs>. The preview follows {follows}.")
    else:
        out.append(f"This release's docs are in the docs preview on the mini, which follows {follows}.")

    def link(slug: str) -> str:
        return f"- [{slug}]({preview}{doc_path(slug)})" if preview else f"- `{doc_path(slug)}`"

    pages = changed_doc_pages(git, base, source)
    if pages:
        out += ["", f"Pages changed on {on} since {since}:", ""]
        out += [link(s) for s in pages]
    if planned:
        out += ["", f"Docs written for planned features, on `{docs_branch}` and not on {on} yet:", ""]
        out += [link(s) for s in planned]
    return out


MARKER = re.compile(r"<!-- kit-release-draft version=(\S+) base=(\S+) source=([0-9a-f]{40}) -->")


def parse_marker(body: str) -> dict | None:
    m = MARKER.search(body or "")
    if not m:
        return None
    return {"version": m.group(1), "base": None if m.group(2) == "-" else m.group(2), "source": m.group(3)}


# --- upsert ---------------------------------------------------------------


def gh_cli(args: list[str], stdin: str | None = None) -> str:
    p = subprocess.run(["gh", *args], input=stdin, capture_output=True, text=True)
    if p.returncode != 0:
        raise DraftError(f"gh {' '.join(args[:3])}: {p.stderr.strip()}")
    return p.stdout


def find_releases(gh, repo: str, draft: Draft) -> list[dict]:
    # One compact JSON object per line; works on gh releases without --slurp.
    out = gh(["api", "--paginate", "--jq", ".[]", f"repos/{repo}/releases?per_page=100"])
    releases = [json.loads(line) for line in out.splitlines() if line.strip()]
    return [r for r in releases
            if r.get("tag_name") == draft.tag
            or (r.get("draft") and r.get("name") == draft.title and str(r.get("tag_name", "")).startswith("untagged-"))]


def upsert(gh, repo: str, draft: Draft) -> str:
    """Create or update the draft. Returns "created", "updated" or "unchanged"."""
    matches = find_releases(gh, repo, draft)
    published = [r for r in matches if not r.get("draft")]
    if published:
        raise DraftError(f"{draft.tag} is already published ({published[0].get('html_url')}); not touching it")
    if len(matches) > 1:
        ids = ", ".join(str(r["id"]) for r in matches)
        raise DraftError(f"more than one draft for {draft.tag} (ids {ids}); delete the extra one")
    payload = {
        # Always send tag_name and target_commitish: a PATCH without them can
        # leave the draft as untagged-…, and the target is what the tag will
        # point at when Adam publishes.
        "tag_name": draft.tag,
        "target_commitish": draft.source,
        "name": draft.title,
        "body": draft.body,
        "draft": True,
        "prerelease": False,
    }
    if not matches:
        gh(["api", "--method", "POST", f"repos/{repo}/releases", "--input", "-"], json.dumps(payload))
        return "created"
    current = matches[0]
    if all(current.get(k) == payload[k] for k in ("tag_name", "target_commitish", "name", "body")):
        return "unchanged"
    # Re-read right before writing: if Adam published in between, stop.
    fresh = json.loads(gh(["api", f"repos/{repo}/releases/{current['id']}"]))
    if not fresh.get("draft"):
        raise DraftError(f"{draft.tag} was published while rendering; not touching it")
    gh(["api", "--method", "PATCH", f"repos/{repo}/releases/{current['id']}", "--input", "-"], json.dumps(payload))
    return "updated"


# --- main -----------------------------------------------------------------


def main(argv: list[str] | None = None, gh=gh_cli) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--ref", default="HEAD", help="commit to render from (default HEAD)")
    ap.add_argument("--version", help="release version X.Y.Z (default: see resolve_version)")
    ap.add_argument("--base", help="last release tag (default: nearest vX.Y.Z tag reachable from --ref)")
    ap.add_argument("--branch", default=os.environ.get("KIT_TRAIN_BRANCH"),
                    help="train branch vX.Y the draft is for (default $KIT_TRAIN_BRANCH); the version is then "
                         "always X.Y.Z. --apply requires it")
    ap.add_argument("--repo", default=DEFAULT_REPO)
    ap.add_argument("--preview-url", default=os.environ.get("KIT_DOCS_PREVIEW_URL"),
                    help="docs preview origin, private (default $KIT_DOCS_PREVIEW_URL; none: pages listed unlinked)")
    ap.add_argument("--docs-branch", default=os.environ.get("KIT_TRAIN_DOCS_BRANCH"),
                    help="train docs branch (default $KIT_TRAIN_DOCS_BRANCH, else the notes file's "
                         "'<!-- train-docs: BRANCH -->' line, else train/vX.Y-docs); read as origin/BRANCH")
    ap.add_argument("--repo-dir", default=".", help="git checkout to read (default .)")
    ap.add_argument("--print-version", action="store_true", help="print only the resolved version")
    ap.add_argument("--json", action="store_true", help="print version, tag, base, source, title and body as JSON")
    ap.add_argument("--published", action="store_true",
                    help="render the public body (hevkit.com docs links, no preview); release.yml uses it after publish")
    ap.add_argument("--apply", action="store_true", help="create or update the draft on GitHub through gh")
    args = ap.parse_args(argv)
    try:
        git = Git(Path(args.repo_dir))
        if args.published and args.apply:
            raise DraftError("--published renders a body for a published release; it never upserts a draft")
        if args.apply and not train_line(args.branch):
            raise DraftError("--apply needs --branch vX.Y: a draft is always for a train branch")
        draft = render(git, args.ref, args.version, args.base, args.branch, args.repo,
                       None if args.published else args.preview_url, args.published, args.docs_branch)
        if args.print_version:
            print(draft.version)
            return 0
        if args.json:
            print(json.dumps({"version": draft.version, "tag": draft.tag, "base": draft.base,
                              "source": draft.source, "title": draft.title, "body": draft.body}, indent=2))
        elif not args.apply:
            sys.stdout.write(draft.body)
        if args.apply and not draft.entries and not draft.declared:
            # A train branch that has not moved since its last tag (a patch
            # line with nothing on it yet) gets no empty draft.
            print(f"release-draft: nothing merged to {args.branch} since {draft.base} and no notes file; "
                  "no draft written", file=sys.stderr)
        elif args.apply:
            result = upsert(gh, args.repo, draft)
            print(f"release-draft: {draft.tag} on {args.repo}: {result} (source {draft.source[:7]})", file=sys.stderr)
        else:
            print(f"release-draft: dry run for {draft.tag}; GitHub not touched (pass --apply to upsert)",
                  file=sys.stderr)
        return 0
    except DraftError as e:
        print(f"release-draft: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
