# Release train

How kit ships a release. The design and its reasons are
[RFC 0014](../rfcs/0014-release-train.md). This replaces the old
`docs/release.md` checklist; every check it had is below (see
[Preflight](#4-preflight) and [Bundled skills](#bundled-skills)).

Two rules hold the train together:

1. **Docs first.** A feature's docs are written and readable in the docs
   preview before or with its implementation, and land on `main` with its
   code.
2. **The draft is the ledger, publishing is the gate.** One draft GitHub
   release per next version accumulates everything merged to `main`.
   **Publishing the draft is Adam's one act.** Everything before it and after
   it is FOH, workers or CI.

```
 merge to main ──► release-draft.yml re-renders draft vX.Y.0     (continuous)
               └─► the mini's preview rebuilds main's docs       (continuous)

 "cut X.Y.Z"
   ├─ 1. scope     the manifest: what rides, what waits           FOH + Adam
   ├─ 2. notes     docs/releases/vX.Y.Z.md final, changelog,      FOH (worker drafts)
   │               install pages name X.Y.Z
   ├─ 3. draft     KIT_RELEASE_DRAFT=on keeps it current          CI
   ├─ 4. preflight release-preflight.yml green + manual checks    FOH
   ├─ 5. publish   Adam reads the draft and the preview, publishes Adam
   ├─ 6. train     release.yml: test, build, image, assets, tap   CI
   ├─ 7. site      site.yml: hevkit.com pinned to vX.Y.Z           CI
   └─ 8. verify    release.yml verify job + a fresh install        CI + FOH
```

## Roles

| Who | Does | Never does |
|---|---|---|
| Worker (dispatched agent) | Writes the feature, its docs page and its line in `docs/releases/vX.Y.Z.md`; stops at a commit (or a draft PR when the order asks). | Push to `main`, merge, touch a release, tag, secret or the preview service. |
| FOH | Pushes, opens and merges PRs; keeps the notes file and the manifest current; turns the draft workflow on; runs the preflight and the manual checks; tells Adam when a draft is ready; reads the verify table and reports. Runs the preview on the mini. | Publish, unpublish or edit the draft body by hand; create or move a tag; change a secret without Adam. |
| Adam | Confirms the manifest and the cut; reads the draft and the docs preview; **publishes the draft**. | — |
| CI | Re-renders the draft on every merge; on publish, builds and ships the release, pins hevkit.com, verifies. | Publish anything on its own. |

## 1. Docs first

Kit's version of Layer's documentation-first rule:

- **Before complex implementation**, the feature's customer-facing docs (who
  it is for, what they run, what they see, how it fails) are written and
  agreed with Adam. Proportionate: a small fix needs only an updated example.
  If implementation changes a promise the docs made, the docs change and Adam
  sees it again.
- **A feature's docs land in the feature's PR.** The page or section lives
  under `site/src/content/docs/`, is linked from `site/src/lib/docs.ts`
  (`docsNav`) when it is a new page, and is built by `site.yml` on the PR.
  A merged feature without docs is a bug; FOH does not merge one.
- **Docs for an unbuilt feature stay on the feature's branch.** They never
  merge ahead of the code. That is how they are kept from production: the only
  docs on `main` are for merged features, the preview shows `main`, and
  production shows only released tags (see [Production](#production-hevkitcom-stays-on-the-latest-release)).
  No feature flags or hidden pages are needed.
- **Each feature has a line in the notes.** The PR that merges a feature adds
  it to `docs/releases/vX.Y.Z.md` under "In this release so far", linking its
  docs page, or FOH adds it right after merging.
- Marketing (`site/src/pages/index.astro`) may lead the release; history and
  future plans go in RFCs and release notes, not the docs.

### The free/paid seam must be stated

From 0.7 the line between free and paid is: **free kit keeps everything on the
file system; paid is adding hev layer, which moves everything off the file
system.** The docs must say this plainly wherever storage, traces, the board,
the dashboard or `hev up` are described: what lives in files with no key, and
what changes when a Layer key is added. The preflight does not check prose, so
FOH reads for it before publishing. This is a docs requirement, not a release
highlight; don't headline it in the notes. (Recorded for the 0.7 docs order.)

## 2. The docs preview

The preview is hevkit.com's site built from `main` and served **on the private
network from the mini**. It is not on Cloudflare and has no public hostname.
It shows what `main` documents, which is what the next release will document.

`scripts/docs-preview.sh` does all of it:

```sh
scripts/docs-preview.sh build [REF]   # build site/ at REF (default origin/main), make it current
scripts/docs-preview.sh refresh       # fetch; rebuild only if origin/main moved
scripts/docs-preview.sh serve         # serve the current build (foreground)
scripts/docs-preview.sh status        # sha, ref and time of the current build
```

Builds go to `~/.cache/kit-docs-preview/builds/<sha>`; `current` is a symlink
swapped only after a build succeeds, so a broken commit leaves the last good
build up, and the server picks up a new build without a restart. Every build
has `/preview.json` naming the commit it was built from.

**On the mini** (FOH sets this up once; nothing here starts it):

```sh
cd ~/workspace/kit-site-preview          # a kit clone used only for the preview
git fetch origin && git checkout --detach origin/main
export KIT_PREVIEW_HOST="$(tailscale ip -4)" KIT_PREVIEW_PORT=4342
scripts/docs-preview.sh build             # first build
scripts/docs-preview.sh serve             # keep running (tmux or a launchd agent)
```

Port 4342 is the one the hand-run `astro preview` on the mini uses today;
stop that process before starting `serve` in its place. Binding to the tailnet
address keeps it off the LAN; anyone on the tailnet reads it at
`http://<mini>:4342/docs`.

**Refreshing when `main` moves:** run `refresh` every five minutes from a
launchd agent (or a tmux loop). It fetches, compares `origin/main` to the
current build's sha, and builds only when they differ, so an idle tick costs a
`git fetch`. One build runs at a time; an overlapping tick skips.

```sh
while sleep 300; do scripts/docs-preview.sh refresh; done
```

The preview follows `main`, which is the minor train. For a patch train,
run a second copy with `KIT_PREVIEW_REF=origin/release/vX.Y`,
`KIT_PREVIEW_PORT=4343` and its own `KIT_PREVIEW_DIR`.

The draft release links the preview when the repository secret
`KIT_DOCS_PREVIEW_URL` holds its address (a secret so the private address
never appears in public logs or the repo). The link is in the draft only:
on publish, `release.yml` rewrites that section to point at hevkit.com.

### Production hevkit.com stays on the latest release

`site.yml` builds production on every `release: published` (and by hand). It
checks out `main` for the site's shell and runs
`site/scripts/pin-docs-release.sh <latest release tag>`, which replaces
`site/src/content/docs` with the tag's copy, so production shows exactly the
docs of the latest published release, and features merged since are not on
it until Adam publishes them. It writes `/docs-release.json` naming the tag;
the verify reads it.

The deploy step runs only when the `KIT_SITE_PAGES_PROJECT` variable and the
`CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` secrets exist. Until then
the job uploads the pinned build as the `hevkit-vX.Y.Z` artifact and FOH
deploys it by hand as today. A release older than 0.7 has no `site/` in its
tag, so the job skips it.

## 3. The draft release

`release-draft.yml` runs on every push to `main` (and to `release/vX.Y`). It
renders the draft body with `scripts/release-draft.py` and shows it in the
run summary. When the repository variable `KIT_RELEASE_DRAFT` is `on`, it
also creates or updates the draft on `hev/kit`:

- **Version.** The lowest `docs/releases/vX.Y.Z.md` above the last tag that
  has no tag yet; without one, the next minor (the next patch on a
  `release/` branch). A notes file is how a train is declared.
- **Body**, a pure function of one commit:
  1. `docs/releases/vX.Y.Z.md`, or "Release notes not written yet";
  2. "Merged since vA.B.C": one line per first-parent commit since the last
     tag, each linking its PR, grouped Changes, Fixes, Docs, Release process,
     Dependencies. Grouping is by rule: dependabot and `Bump …` are
     dependencies; `fix:`/`Fix …` titles are fixes; PRs touching only
     `docs/` or `site/` are docs; only `docs/releases/` is release process.
     A `Release-Kind: changes|fixes|docs|release|dependencies|skip` trailer
     in the merge commit overrides it;
  3. "Docs preview": every `site/src/content/docs` page added or changed
     since the last tag, linked into the preview when
     `KIT_DOCS_PREVIEW_URL` is set;
  4. the marker `<!-- kit-release-draft version=… base=… source=<sha> -->`.
- **Target.** The draft's `target_commitish` is the exact commit it was
  rendered from. Publishing creates the tag there, so what ships is exactly
  what Adam read, even if `main` moved since.
- **Idempotent.** Re-rendering an unchanged commit gives the same bytes and
  no write. A run for a commit that is no longer the branch tip writes nothing.
  The upsert always sends `tag_name` and `target_commitish` (a body-only edit
  can leave a draft `untagged-…`), restores an `untagged-…` draft named
  `kit vX.Y.Z`, refuses to choose between two drafts, re-reads before
  writing, and never touches a published release.

By hand (FOH):

```sh
scripts/release-draft.py                 # print the body; touches nothing
scripts/release-draft.py --version 0.7.0 --json
scripts/release-draft.py --apply         # create/update the draft (needs gh as a maintainer)
```

**Don't edit the draft body on GitHub.** The next merge replaces it. To change
the notes, change `docs/releases/vX.Y.Z.md`.

## 4. Notes, scope and prep

`docs/releases/vX.Y.Z.md` is written for someone reading the release:
what they can do now, the command, a link to the docs page, the PR. Shape:
a short lead list of what shipped, then "Fixed", "Removed", "Build" as
needed. Shipped work only. During a train it also has a
**"Planned for this train"** section: the manifest, kept apart from what has
merged. Don't describe unmerged work as shipped.

Cutting `X.Y.Z` (FOH, after Adam confirms scope):

1. **Manifest.** For each planned item: merged with its docs (move it up into
   the shipped part), or not ready by the cut (it waits for the next train).
   Then **delete the "Planned" section**; the preflight refuses a notes file
   that still has one.
2. **Changelog.** Add `## X.Y.Z` to `site/src/content/docs/changelog.mdx`
   (date, link to the release, the shipped items).
3. **Docs pin.** The pages that name the current release (`install.mdx`,
   `index.mdx`, `windows.mdx`, `invited.mdx`, `join-a-host.mdx`) name `X.Y.Z`
   and only `X.Y.Z`. The preflight checks every version on them.
4. Merge that as one `[docs]` PR. The draft re-renders from it.

There is no version-bump commit: kit's version is injected at build time from
the tag (`-X github.com/hev/kit/internal/version.Version`), and the dashboard
image `hev up` pulls is `hevlayer/kit:<that version>`
(`internal/version.KitImage`). The tag is the version.

## 5. Preflight

```sh
gh workflow run release-preflight.yml -f version=X.Y.Z
# or locally, from a checkout of the draft's source commit:
scripts/release-check.py preflight X.Y.Z --draft [--docker]
```

It checks out the draft's source commit and writes a pass/fail table. **A
green preflight and the manual checks below are the "ready to publish"
signal.** FOH then tells Adam, with the draft link.

| Area | Check |
|---|---|
| source | tag `vX.Y.Z` does not exist yet |
| notes | `docs/releases/vX.Y.Z.md` exists, is not a placeholder, has no "Planned" section |
| go | `go vet ./...`, `GOOS=windows go vet ./...`, `go test ./...` |
| build | `.github/build-release.sh` builds all five archives (darwin and linux arm64/amd64, the Windows zip) with the version injected; the host's binary prints `X.Y.Z` for `hev version` |
| skills | in a temporary HOME with `.claude` and `.codex`, the candidate's `hev skills setup` then `hev skills check`: every bundled skill current |
| image | `internal/version` test: a release binary pins `hevlayer/kit:X.Y.Z`; that tag is not on Docker Hub yet; the dashboard image builds with `VERSION=X.Y.Z` and its `hev version` prints it (`--docker`) |
| docs | `changelog.mdx` has `## X.Y.Z`; the install and download pages name only `X.Y.Z` |
| draft | exactly one release `vX.Y.Z`, still a draft, whose marker names this version and whose source is the target commit and this checkout; notes rendered |

Manual checks (from the old checklist; FOH, recorded on the notes PR or in
the "ready" message):

- **One browser validation pass.** Run `hev validate NAME` against a completed
  browser order and inspect its verdict and acceptance evidence with
  `hev dispatch show NAME`. Resolve failures before publishing.
- **Bundled skills reviewed** for public content and shipped commands: read the
  `skills/` diff since the last tag.
- **The free/paid seam** reads clearly in the preview (see
  [Docs first](#the-freepaid-seam-must-be-stated)).

## 6. Publish (Adam)

Read the draft on `hev/kit` (notes and ledger) and the docs preview. If it is
right, press **Publish release**. Leave "Set as latest release" ticked for a
new minor or a patch on the newest line.

Publishing creates the tag `vX.Y.Z` at the draft's target commit and fires
`release: published`. That event, not a tag push, starts `release.yml`.
Pushing a tag by hand no longer releases anything.

Merges to `main` after publishing ride the next train.

## 7. Train (`release.yml`)

Started by the publish, as Adam. In order: refuses a draft or a non-`vX.Y.Z`
tag; checks out the tag; `go test`; builds the archives; pushes
`hevlayer/kit:X.Y.Z` (and `:latest` only if this is the newest release);
uploads the archives and `checksums.txt` to the release (`--clobber`, so a
re-run replaces them); rewrites the release body for the public (the private
preview section becomes hevkit.com links; the notes above it are re-rendered
from the same commit and are unchanged); bumps the tap (newest release only).
The tap goes last: a formula must never point at assets that are not there.

Between publish and the upload step (a few minutes) the release page has no
assets; Homebrew users are unaffected because the tap moves last.

A failed step: fix the cause and re-run the failed job, or
`gh workflow run release.yml -f tag=vX.Y.Z`. Every step is safe to repeat.

`site.yml` fires on the same publish and rebuilds hevkit.com with the docs
pinned to the new tag (see [Production](#production-hevkitcom-stays-on-the-latest-release)).

## 8. Verify

`release.yml`'s `verify` job runs `scripts/release-check.py verify X.Y.Z`
(read-only; re-run by hand any time):

| Check | Passes when |
|---|---|
| release | `vX.Y.Z` published; five archives and `checksums.txt` attached; it is the latest release (newest line) |
| install | the host's archive matches `checksums.txt`, and the downloaded `hev version` prints `X.Y.Z` |
| image | `hevlayer/kit:X.Y.Z` is on Docker Hub; `:latest` is the same digest (newest line) |
| tap | `hev/homebrew-tap` `Formula/hev.rb` and `kit.rb` serve `X.Y.Z` (newest line) |
| docs | `hevkit.com/docs-release.json` names `vX.Y.Z` (skipped until production deploys from CI) |

FOH also runs `brew update && brew upgrade hev/tap/hev && hev version` on one
Mac, then reports to Adam.

## Patch releases

A fix for a published `X.Y`:

- **On the newest line** (nothing new on `main` you'd hold back): it is just
  the next train with a patch number. Add `docs/releases/vX.Y.(Z+1).md` on
  `main`; the draft workflow then drafts `vX.Y.(Z+1)` instead of the next
  minor. Same prep, preflight and publish.
- **When `main` has moved on**: branch `release/vX.Y` from the `vX.Y.Z` tag,
  cherry-pick the fix (it must be on `main` first), add
  `docs/releases/vX.Y.(Z+1).md` on the branch. Pushes to `release/vX.Y` keep
  draft `vX.Y.(Z+1)` current, targeting the branch. Preflight with
  `-f ref=release/vX.Y`. If it is not the newest release, Adam unticks "Set as
  latest"; the train then leaves `:latest` and the tap alone.

## Setup the train needs

| Thing | Where | Status |
|---|---|---|
| `KIT_RELEASE_DRAFT=on` | repository variable | FOH sets it to start keeping the draft; until then every run is a dry run |
| `KIT_DOCS_PREVIEW_URL` | repository secret | optional; the preview's private address, for links in the draft |
| `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, `HOMEBREW_TAP_TOKEN` | repository secrets | exist (used by every release so far) |
| `KIT_SITE_PAGES_PROJECT`; `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` | variable; secrets | optional; without them production is deployed by hand from the `site.yml` artifact |
| The preview on the mini | `scripts/docs-preview.sh serve` + `refresh` loop | FOH starts it |

## Bundled skills

Previously only `hev-traces` was installed. The binary now embeds `hev-traces`,
`hev-dispatch`, `hev-board`, `hev-peers`, `hev-handoff`, `hev-expedite` and
`hev-member`. `hev skills setup`, `hev up`, `hev join` and `hev skills sync`
share one install path into existing `~/.claude/skills` and `~/.codex/skills`
directories, plus `~/.agents/skills` when pi (`~/.pi`) or OpenCode
(`~/.config/opencode`) is present or hev's stamped copies are already there. Front matter `metadata.audience` (`owner`,
`member` or `all`; omitted means all) selects skills by dispatch access: a
machine whose every dispatch host is a joined node gets the member set, and
owned copies of skills outside its audience are removed. Codex reads
`~/.agents/skills` as well as `~/.codex/skills` and lists a same-name skill from
both twice, so the shared copy is written first and an owned Codex copy is
removed only once the shared copy of that skill is in place. A skills root that
resolves (through symlinks) into another harness's root is written once and
never retired from.
Each has a `.kit-skill.json` sidecar with the kit
version and SHA-256 of SKILL.md. `hev s` and `hev skills check` report missing,
stale, locally edited, or current copies. Sync repairs missing and stale copies.
Unstamped copies matching historical kit content are stale and repaired. Unknown
unstamped copies are reported as not installed by hev and are never replaced or
removed, even with `--force`; rename or remove one to get hev's copy. Symlinks are preserved unless forced; forcing replaces
the link itself and does not write to its target.

Historical unstamped traces copies are recognized by the embedded
`skills/hev-traces/historical-sha256.txt` allowlist, derived from SHA-256 hashes
of `skills/hev-traces/SKILL.md` blobs in git history. Preserve those hashes when
updating the skill so upgrades can distinguish shipped copies from local edits.
`hev skills check` exits non-zero for any missing, stale, or locally edited copy.
