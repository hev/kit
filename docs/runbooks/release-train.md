# Release train

How kit ships a release. The design and its reasons are
[RFC 0014](../rfcs/0014-release-train.md). This replaces the old
`docs/release.md` checklist; every check it had is below (see
[Preflight](#5-preflight) and [Bundled skills](#bundled-skills)).

Four rules hold the train together:

1. **A train branch.** Each train has a branch `vX.Y`, opened from `main`.
   Features for the train merge into `vX.Y`. **`main` does not move between
   releases**: it is the latest published release. After a release ships,
   `main` is fast-forwarded to its tag, never forced, and the next train
   branch opens.
2. **Docs first.** A train's features are documented on its **train docs
   branch**, readable in the docs preview, before they are built. Each
   feature's pages move into `vX.Y` in the feature's PR, so `vX.Y`'s docs only
   ever describe what `vX.Y` does.
3. **The draft is the ledger, publishing is the gate.** One draft GitHub
   release per train accumulates everything merged to `vX.Y`. **Publishing the
   draft is Adam's one act.** Everything before it and after it is FOH,
   workers or CI.
4. **Edge on the operator's cluster.** Adam's own machines run a build of
   `vX.Y`'s tip, so every merged feature does real work before it is
   published. Users install releases.

```
 train opens ──► FOH creates vX.Y from main (after a release: advance does it)
             └─► FOH opens train/vX.Y-docs (draft PR against vX.Y, never merged);
                 docs for each planned feature are written there first;
                 the mini's preview follows it
 feature PR (base vX.Y) ──► carries its pages from the train docs branch
 merge to vX.Y ──► release-draft.yml re-renders draft vX.Y.0     (continuous)
               ├─► FOH rebases the train docs branch on vX.Y
               ├─► the mini's preview rebuilds                   (continuous)
               └─► the operator's cluster moves to the new edge build

 "cut X.Y.Z"
   ├─ 1. scope     the manifest: what rides, what waits;          FOH + Adam
   │               train docs branch closed, preview → vX.Y
   ├─ 2. prep      notes final, changelog, install pages,        FOH (worker drafts)
   │               Dependabot → vX.(Y+1); one PR into vX.Y
   ├─ 3. draft     KIT_RELEASE_DRAFT=on keeps it current          CI
   ├─ 4. preflight release-preflight.yml green + manual checks    FOH
   ├─ 5. publish   Adam reads the draft and the preview, publishes Adam
   ├─ 6. train     release.yml: test, build, image, assets, tap   CI
   ├─ 7. site      site.yml: hevkit.com pinned to vX.Y.Z           CI
   ├─ 8. verify    release.yml verify job + a fresh install        CI + FOH
   └─ 9. advance   main fast-forwards to vX.Y.Z; vX.(Y+1) opens    CI (or FOH)
```

## Roles

| Who | Does | Never does |
|---|---|---|
| Worker (dispatched agent) | A docs worker writes planned features' docs on the train docs branch. A feature worker (order `base: vX.Y`) builds to those docs, moves its pages into its PR, and adds its line to `docs/releases/vX.Y.Z.md`. Stops at a commit (or a draft PR when the order asks). | Push to `main` or `vX.Y`, merge, touch a release, tag, secret or the preview service. |
| FOH | Pushes, opens and merges PRs into `vX.Y`. Creates `vX.Y` when Adam confirms a manifest (if `advance` has not). Dispatches the train's orders with `base: vX.Y`. Opens the train docs branch and its PR; rebases it on `vX.Y` after each feature merge; closes it at the cut. Points the preview at each stage's ref. Keeps the notes file and the manifest current; turns the draft workflow on; runs the preflight and the manual checks; tells Adam when a draft is ready; reads the verify table; runs the `advance` pushes by hand until they are automated; merges `vX.Y` into the next train after a patch; reports. | Publish, unpublish or edit the draft body by hand; create or move a tag; merge anything into `main` or force-push it; change a secret or branch protection without Adam; merge the train docs branch; stop a preview or tmux session it did not create. |
| Adam | Confirms the manifest and the cut; reads and agrees the planned docs in the preview before features are built; reads the draft; **publishes the draft**. | — |
| CI | Re-renders the draft on every merge to `vX.Y`; on publish, builds and ships the release, pins hevkit.com, verifies, fast-forwards `main` and opens the next train (when enabled). | Publish anything on its own; force any branch. |

## 0. The train branch

- **Name**: `vX.Y` (e.g. `v0.7`). It carries `X.Y.0` and afterwards the
  `X.Y.Z` patches. There is no `release/vX.Y`.
- **Opened** from `main` when the train opens. After a release, the `advance`
  job (or FOH) opens `vX.(Y+1)` at the release tag, which is where `main` now
  is. Create it by hand only from `main`:
  `git push origin origin/main:refs/heads/vX.Y`.
- **Feature PRs target `vX.Y`.** Dispatch orders for the train say
  `base: vX.Y`. A PR opened against `main` by mistake is retargeted
  (`gh pr edit N --base vX.Y`); nothing merges into `main` by PR.
- **CI** (`ci`, `adapters`, `site`) runs on pushes to and PRs into `vX.Y` as
  for `main`. That matters twice: features are tested where they merge, and
  the release tag's commit already has the green `go` check `main`'s
  protection requires when it is fast-forwarded.
- **`main` is an ancestor of `vX.Y`**, always; it is what lets `main`
  fast-forward. It breaks only when a patch on the previous line moves `main`
  while `vX.Y` is open: then FOH merges the patch line (`vX.(Y-1)`) into
  `vX.Y` through a PR (a merge commit, not a rebase). Check:
  `git merge-base --is-ancestor origin/main origin/vX.Y && echo ok`. The
  draft workflow's summary warns, and the preflight fails, until it holds.
- **Dependabot** reads its config from `main` only, so the config for a train
  ships in the release before it: at the cut of `X.Y.0`, every update in
  `.github/dependabot.yml` gets `target-branch: "vX.(Y+1)"` (the preflight
  checks it). Security updates always target `main`: don't merge one there;
  make the same bump on `vX.Y` (or on a patch line, and ship a patch) and
  close it.
- **0.7, honestly.** `v0.7` was branched from `main` at a7075f0, which is
  already ahead of v0.6.2 (the merges since that tag, plus #129 and #211).
  Until 0.7.0 ships, `main` is not exactly the last release. It is not
  rewound; it fast-forwards to v0.7.0. Dependabot still targets `main` during
  0.7: hold its PRs (or re-do a bump on `v0.7`) until 0.7.0 ships the
  `target-branch: "v0.8"` config.

## 1. Docs first

Kit's version of Layer's documentation-first rule:

- **Before complex implementation**, the feature's customer-facing docs (who
  it is for, what they run, what they see, how it fails) are written on the
  train docs branch and agreed with Adam in the preview. Proportionate: a
  small fix needs only an updated example. If implementation changes a promise
  the docs made, the docs change and Adam sees it again.
- **A feature's docs land on `vX.Y` in the feature's PR.** The page or
  section lives under `site/src/content/docs/`, is linked from
  `site/src/lib/docs.ts` (`docsNav`) when it is a new page, and is built by
  `site.yml` on the PR. A merged feature without docs is a bug; FOH does not
  merge one.
- **Docs for an unbuilt feature never reach `vX.Y`.** They live on the train
  docs branch until the feature merges. That is how they are kept from
  production: production shows only released tags (see
  [Production](#production-hevkitcom-stays-on-the-latest-release)). No feature
  flags or hidden pages are needed.

### The train docs branch

Each train has one branch carrying the docs for its planned features, so Adam
can read them in the preview before they are built.

- **Name**: `train/vX.Y-docs`, with a draft PR **against `vX.Y`** titled
  "vX.Y train docs (do not merge)". 0.7's predates the convention:
  `d/kit-docs-07-20261111`, PR #210 (now against `v0.7`). A notes file that
  uses another name says so on its own line: `<!-- train-docs: BRANCH -->`
  (0.7's does). The draft generator and the preflight read that line, else the
  convention.
- **Opened** by FOH when the train opens, from `vX.Y`. A docs worker (order
  `base:` the train docs branch) writes each planned feature's pages and
  sections there.
- **Unmarked.** Planned pages are written as the shipped docs will read: no
  "Coming in X.Y" label, callout or table note, here or anywhere else in the
  docs. What keeps them off production is that this branch never merges and
  production hevkit.com is pinned to the last release. The preview is the one
  place they show, and it is private.
- **As each feature merges into `vX.Y`**, its pages move: the feature's PR
  carries them over from the train docs branch (copy the files, or
  cherry-pick the branch's commits for that feature). After FOH merges the feature, FOH rebases
  the train docs branch on `vX.Y` the same day, dropping what is now on
  `vX.Y`, and force-pushes it (the train docs branch is the one branch that is
  force-pushed). The preview picks the rebase up on its next refresh; the
  draft re-renders on that push.
- **At the cut** (step 1 of the cut), every planned feature has either merged
  with its pages or waits for the next train. FOH moves a waiting feature's
  pages to the next train's docs branch (or drops them), closes the PR,
  deletes the branch, and points the preview at `origin/vX.Y`. The preflight
  fails while the branch is still on origin.
- Never merged, never the base of a feature branch (so a feature PR never
  drags in another feature's planned docs).
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

The preview is hevkit.com's site built from one ref and served **on the
private network from the mini**. It is not on Cloudflare and has no public
hostname. What it follows depends on the stage:

| Stage | `KIT_PREVIEW_REF` | `KIT_PREVIEW_FALLBACK` | It shows |
|---|---|---|---|
| Train open | `origin/<train docs branch>` (0.7: `origin/d/kit-docs-07-20261111`) | `origin/vX.Y` | `vX.Y`'s docs plus every planned feature's docs |
| A feature merges into `vX.Y` | unchanged; FOH rebases the docs branch on `vX.Y` | unchanged | the feature's pages, now from `vX.Y`; the rest still from the docs branch |
| Cut onward | `origin/vX.Y` | — | what the release documents, and nothing more |
| Next train opens | `origin/train/vX.(Y+1)-docs` | `origin/vX.(Y+1)` | the next train |

When the followed branch is deleted at the cut, `refresh` falls back to
`KIT_PREVIEW_FALLBACK` by itself and logs it (to `origin/main` if that is
unset). **The step at the cut** is to change `KIT_PREVIEW_REF` to
`origin/vX.Y` in the refresh agent's environment, so the log stops saying it
fell back. Patch lines rarely need a preview; if one does, run a second copy
on its own port and state directory following `origin/vX.Y`.

`scripts/docs-preview.sh` does all of it:

```sh
scripts/docs-preview.sh build [REF]   # build site/ at REF (default $KIT_PREVIEW_REF), make it current
scripts/docs-preview.sh refresh       # fetch; rebuild only if $KIT_PREVIEW_REF moved
scripts/docs-preview.sh serve         # serve the current build (foreground)
scripts/docs-preview.sh status        # sha, ref and time of the current build
```

Builds go to `$KIT_PREVIEW_DIR/builds/<sha>` (default under `~/.cache`);
`current` is a symlink swapped only after a build succeeds, so a broken commit
leaves the last good build up, and the server picks up a new build without a
restart. Every build has `/preview.json` naming the commit it was built from.

**On the mini it runs under launchd**, as two user agents that run the
script from a kit clone used only for the preview:

- **serve**: `ProgramArguments` = the script and `serve`; `KeepAlive` true,
  `RunAtLoad` true. Environment: `KIT_PREVIEW_HOST` (the mini's tailnet
  address, so it is off the LAN), `KIT_PREVIEW_PORT=4370`, and
  `KIT_PREVIEW_DIR`.
- **refresh**: `ProgramArguments` = the script and `refresh`;
  `StartInterval` 300. Environment: the same `KIT_PREVIEW_DIR`, plus
  `KIT_PREVIEW_REF` and `KIT_PREVIEW_FALLBACK` for the stage (table above).

launchd agents get a minimal `PATH`: set one in `EnvironmentVariables` that
reaches `git`, `python3`, `node` and `pnpm`. Changing a stage is editing the
refresh agent's `KIT_PREVIEW_REF` (and fallback) and reloading it
(`launchctl bootout`, then `launchctl bootstrap gui/$UID <plist>`); serve
keeps running. The plists hold a machine's paths and address, so they live on
the mini, not in this repository. 0.7's agents are already running.

Anyone on the tailnet reads the preview at
`http://<the mini's MagicDNS name>:4370/docs` (`serve` prints the name) or
the tailnet IP. Python's `http.server` has no Host check, so both work;
`scripts/test-docs-preview.py` checks a 200 for a MagicDNS Host header.

The draft release links the preview when the repository secret
`KIT_DOCS_PREVIEW_URL` holds its address (a secret so the private address
never appears in public logs or the repo). The link is in the draft only:
on publish, `release.yml` rewrites that section to point at hevkit.com.
Because the preview follows the train docs branch, which is `vX.Y` plus the
planned docs, the same origin serves both shipped and planned pages; the
draft links each list into it (see [The draft release](#3-the-draft-release)).

### Production hevkit.com stays on the latest release

`site.yml` builds production on every `release: published` (and by hand)
from the latest published release's tag:
`site/scripts/pin-docs-release.sh <tag>` pins `site/src/content/docs` to it
and writes `/docs-release.json` naming the tag; the verify reads it.
Unreleased work lives only on train branches, so it cannot reach production.

The deploy step runs only when the `KIT_SITE_PAGES_PROJECT` variable and the
`CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` secrets exist. Until then
the job uploads the pinned build as the `hevkit-vX.Y.Z` artifact and FOH
deploys it by hand as today. A release older than 0.7 has no `site/` in its
tag, so the job skips it.

## 3. The draft release

`release-draft.yml` runs on every push to a train branch `vX.Y` and to its
train docs branch `train/vX.Y-docs` (which re-renders `vX.Y`'s draft), and by
hand (`gh workflow run release-draft.yml -f branch=vX.Y`; 0.7's docs branch
does not match the pattern, so FOH runs this after rebasing it). It renders
the draft body with `scripts/release-draft.py` and shows it in the run
summary, with a warning when `main` is not an ancestor of `vX.Y`. When the
repository variable `KIT_RELEASE_DRAFT` is `on`, it also creates or updates
the draft on `hev/kit`:

- **Version.** Always `X.Y.Z` for branch `vX.Y`: the lowest untagged
  `docs/releases/vX.Y.Z.md` above the last tag; else `X.Y.0` for a new train;
  else, after `X.Y.0` shipped, the next patch. A patch line with nothing
  merged since its last tag and no notes file gets no draft.
- **Body**, a pure function of one commit:
  1. `docs/releases/vX.Y.Z.md`, or "Release notes not written yet";
  2. "Merged since vA.B.C": one line per first-parent commit since the last
     tag, each linking its PR, grouped Changes, Fixes, Docs, Release process,
     Dependencies. Grouping is by rule: dependabot and `Bump …` are
     dependencies; `fix:`/`Fix …` titles are fixes; PRs touching only
     `docs/` or `site/` are docs; only `docs/releases/` is release process.
     A `Release-Kind: changes|fixes|docs|release|dependencies|skip` trailer
     in the merge commit overrides it;
  3. "Docs preview": what the preview follows, then every
     `site/src/content/docs` page added or changed on `vX.Y` since the last
     tag, then, while the train docs branch exists, the pages it adds or
     changes that are not on `vX.Y` yet ("Docs written for planned
     features"). Both lists link into the preview when `KIT_DOCS_PREVIEW_URL`
     is set. The branch is the notes file's `<!-- train-docs: … -->` line,
     else `train/vX.Y-docs`, read as `origin/<branch>`; `--docs-branch`
     overrides;
  4. the marker `<!-- kit-release-draft version=… base=… source=<sha> -->`.
- **Target.** The draft's `target_commitish` is the exact `vX.Y` commit it
  was rendered from. Publishing creates the tag there, so what ships is
  exactly what Adam read, even if `vX.Y` moved since.
- **Idempotent.** Re-rendering an unchanged commit gives the same bytes and
  no write. A run for a commit that is no longer the branch tip writes nothing.
  The upsert always sends `tag_name` and `target_commitish` (a body-only edit
  can leave a draft `untagged-…`), restores an `untagged-…` draft named
  `kit vX.Y.Z`, refuses to choose between two drafts, re-reads before
  writing, and never touches a published release.

By hand (FOH):

```sh
scripts/release-draft.py --ref origin/v0.7 --branch v0.7          # print the body; touches nothing
scripts/release-draft.py --ref origin/v0.7 --branch v0.7 --json
scripts/release-draft.py --ref origin/v0.7 --branch v0.7 --apply  # create/update the draft (gh as a maintainer)
```

**Don't edit the draft body on GitHub.** The next merge replaces it. To change
the notes, change `docs/releases/vX.Y.Z.md` on `vX.Y`.

## 4. Notes, scope and prep

`docs/releases/vX.Y.Z.md` is written for someone reading the release:
what they can do now, the command, a link to the docs page, the PR. Shape:
a short lead list of what shipped, then "Fixed", "Removed", "Build" as
needed. Shipped work only. During a train it also has a
**"Planned for this train"** section: the manifest, kept apart from what has
merged. Don't describe unmerged work as shipped.

Cutting `X.Y.Z` (FOH, after Adam confirms scope):

1. **Manifest.** For each planned item: merged into `vX.Y` with its docs
   (move it up into the shipped part), or not ready by the cut (it waits for
   the next train; retarget its PR when `vX.(Y+1)` opens). Then **delete the
   "Planned" section**; the preflight refuses a notes file that still has
   one. **Close the train docs branch**: move a waiting feature's pages to the
   next train's docs branch, close the PR, delete the branch, and set the
   preview's `KIT_PREVIEW_REF=origin/vX.Y`.
2. **Changelog.** Add `## X.Y.Z` to `site/src/content/docs/changelog.mdx`
   (date, link to the release, the shipped items).
3. **Docs pin.** The pages that name the current release (`install.mdx`,
   `index.mdx`, `windows.mdx`, `invited.mdx`, `join-a-host.mdx`) name `X.Y.Z`
   and only `X.Y.Z`. The preflight checks every version on them.
4. **Dependabot** (`X.Y.0` only): every update in `.github/dependabot.yml`
   gets `target-branch: "vX.(Y+1)"`.
5. Merge that as one prep PR into `vX.Y`. The draft re-renders from it.

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

It checks out the draft's source commit (else the tip of `vX.Y`) and writes a
pass/fail table. **A green preflight and the manual checks below are the
"ready to publish" signal.** FOH then tells Adam, with the draft link.

| Area | Check |
|---|---|
| source | tag `vX.Y.Z` does not exist yet; for `X.Y.0`, Dependabot targets `vX.(Y+1)` |
| lineage | the commit is on `origin/vX.Y`, and `origin/main` is its ancestor (so `main` can fast-forward to the tag) |
| notes | `docs/releases/vX.Y.Z.md` exists, is not a placeholder, has no "Planned" section |
| go | `go vet ./...`, `GOOS=windows go vet ./...`, `go test ./...` |
| build | `.github/build-release.sh` builds all five archives (darwin and linux arm64/amd64, the Windows zip) with the version injected; the host's binary prints `X.Y.Z` for `hev version` |
| skills | in a temporary HOME with `.claude` and `.codex`, the candidate's `hev skills setup` then `hev skills check`: every bundled skill current |
| image | `internal/version` test: a release binary pins `hevlayer/kit:X.Y.Z`; that tag is not on Docker Hub yet; the dashboard image builds with `VERSION=X.Y.Z` and its `hev version` prints it (`--docker`) |
| docs | `changelog.mdx` has `## X.Y.Z`; the install and download pages name only `X.Y.Z`; the train docs branch is gone from origin |
| draft | exactly one release `vX.Y.Z`, still a draft, whose marker names this version and whose source is the target commit and this checkout; notes rendered |

Manual checks (from the old checklist; FOH, recorded on the prep PR or in
the "ready" message):

- **One browser validation pass.** Run `hev validate NAME` against a completed
  browser order and inspect its verdict and acceptance evidence with
  `hev dispatch show NAME`. Resolve failures before publishing.
- **Bundled skills reviewed** for public content and shipped commands: read the
  `skills/` diff since the last tag.
- **The free/paid seam** reads clearly in the preview (see
  [Docs first](#the-freepaid-seam-must-be-stated)).
- **Edge has run it.** The operator's cluster has been on an edge build of
  the release commit (or one close to it) for real work, and
  `hev dispatch hosts` shows no host behind it.

## 6. Publish (Adam)

Read the draft on `hev/kit` (notes and ledger) and the docs preview. If it is
right, press **Publish release**. Leave "Set as latest release" ticked for a
new minor or a patch on the newest line.

Publishing creates the tag `vX.Y.Z` at the draft's target commit on `vX.Y`
and fires `release: published`. That event, not a tag push, starts
`release.yml`. Pushing a tag by hand no longer releases anything.

Merges to `vX.Y` after publishing are patches for `X.Y.(Z+1)`; new features
go to the next train.

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

`site.yml` fires on the same publish and rebuilds hevkit.com from the new tag
(see [Production](#production-hevkitcom-stays-on-the-latest-release)).

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

## 9. Advance: `main` to the tag, the next train opens

`release.yml`'s `advance` job runs after `verify`, for the newest release
only: `scripts/release-advance.py X.Y.Z`.

- **`main` fast-forwards to the tag.** A plain `git push` of the tag's commit
  to `main`, never forced. If `main` is not an ancestor of the tag it
  **stops** (the job fails); FOH reports to Adam and changes nothing. (The
  fix is a merge of `main` into `vX.Y` through a PR and a patch release; `main`
  is never forced or rewound.)
- **The next train opens.** For `X.Y.0`, `vX.(Y+1)` is created at the tag
  unless it exists. If it exists, or after a patch on the newest line, it must
  contain the tag; when it does not, the job warns, and FOH merges `vX.Y` into
  `vX.(Y+1)` through a PR.
- **Token.** `main` requires the `go` check, and the tag commit already
  passed it on `vX.Y`. `GITHUB_TOKEN` cannot push a `main` update that brings
  in `.github/workflows/` changes (it has no workflows permission), so the job
  checks out with the `KIT_MAIN_PUSH_TOKEN` secret when it exists: a
  fine-grained token on `hev/kit` with Contents and Workflows read/write,
  owned by someone allowed to push to `main`.
- **Dry run until enabled.** Until the repository variable
  `KIT_ADVANCE_MAIN=on`, the job prints the pushes and FOH runs them as a
  maintainer:

  ```sh
  scripts/release-advance.py X.Y.Z           # what it would push
  scripts/release-advance.py X.Y.Z --apply   # push: main fast-forward, open vX.(Y+1)
  ```

Then FOH: retargets the PRs that wait for the next train to `vX.(Y+1)`, opens
`train/vX.(Y+1)-docs`, moves the preview to it (fallback `origin/vX.(Y+1)`),
and moves the operator's cluster to the new edge.

## Patch releases

One scheme: a patch for `X.Y` rides `vX.Y`.

- **A fix for the newest line** merges into `vX.Y` (by PR, base `vX.Y`). If
  it applies to the open train too, it also goes into `vX.(Y+1)` (merge `vX.Y`
  into it, below). Add `docs/releases/vX.Y.(Z+1).md` on `vX.Y`; the draft
  workflow drafts `vX.Y.(Z+1)` from `vX.Y`. Same preflight and publish.
  `advance` then fast-forwards `main` to the patch tag and warns that
  `vX.(Y+1)` lacks it: **FOH merges `vX.Y` into `vX.(Y+1)` through a PR**, so
  `main` stays an ancestor of the open train.
- **A fix for an older line** (`v0.6` after 0.7.0): create `vX.Y` from its
  last tag if it does not exist (`git push origin vX.Y.Z^{commit}:refs/heads/vX.Y`),
  merge the fix there, and release as above. Adam unticks "Set as latest";
  the train then leaves `:latest`, the tap and `main` alone.

## Edge: the operator's cluster runs the train

Adam's cluster "runs edge across everything". **Edge** is the tip of the open
train branch `vX.Y`. Every machine of the operator's own (each host that sends
or receives dispatch, its capture daemon, and the kit dashboard) runs a build
of it, not the last release, so each merged feature does real work before the
draft is published. A release is then a promotion of what has already been
running. The commands are documented for everyone on
[Updating and channels](https://hevkit.com/docs/updating); this is how the
train uses them.

**Edge builds come from CI.** `edge.yml` runs on every push to `vX.Y`: it runs
`go test`, builds the five archives with `.github/build-release.sh` at version
`X.Y.Z-edge.<short sha>` (X.Y.Z is the version the draft is for, so `X.Y.0`
on a new train), writes `manifest.json` (version, train, commit, commit count
as `sequence`, each archive's sha256) with `scripts/edge-build.py manifest`,
attests build provenance, and publishes them with `checksums.txt` as a
pre-release tagged `edge-X.Y.Z-edge.<sha>`. It keeps the newest 10 per train
and deletes older ones with their tags. It never marks one latest, pushes no
image and does not touch the tap; the tag does not start with `v`, so
`release-draft.py`'s last-release glob never sees it; and it is never a
draft, so the train's draft is untouched. `release.yml` and `site.yml` skip
pre-releases. It needs only `GITHUB_TOKEN` with `contents: write` (and
`id-token`/`attestations: write` for provenance), declared in the workflow.
By hand: `scripts/edge-build.py version --branch vX.Y` prints the version for
the checkout; `prune --repo hev/kit --train vX.Y` (without `--apply`) lists
what would be deleted.

**Everything means:**

- the `hev` on `PATH` on every sender and every target, which is also the
  binary supervisors, plugins and validators exec;
- `hevd`, the capture and dispatch daemon on each host;
- the bundled skills in each harness;
- the kit dashboard service (`hev up`'s image), built from the same commit.

`hev update` moves the first three. The dashboard image is not published for
edge builds: rebuild it from the same commit with `VERSION=X.Y.Z-edge.<sha>`,
tagged `hevlayer/kit:X.Y.Z-edge.<sha>` locally (the tag the edge binary's
`hev up` asks for), and run `hev up`.

**Moving the cluster to the newest edge build:**

```sh
hev dispatch hosts                       # VERSION, CHANNEL, BEHIND per host
hev dispatch hosts update --all --check  # what would change
hev dispatch hosts update --all          # each host moves itself; BEFORE and AFTER per host
hev update                               # this machine, if it is not one of its own hosts
```

Each host fetches the build from the repository, checks its sha256 against
the manifest, installs it under `~/.hev/versions/`, switches the `hev` it
runs (and `~/.local/bin/hev`) atomically, restarts `hevd` through launchd,
the systemd user unit or the scheduled task, and runs `hev skills sync` from
the new binary. Running supervisors keep the binary they started with; new
runs start on the new one. A host whose daemon does not come back switches
back and says so. Hosts can instead follow edge by themselves: `auto = true`
under `[update]` in each host's `~/.hev/config.toml`.

**The first move from 0.6.2.** A 0.6.2 host has no `hev update` and answers
`hev dispatch hosts update` with an error naming the manual step. Move each
host once by hand (download the newest edge archive and `checksums.txt`,
check it, install it over the `hev` on its PATH and `~/.local/bin/hev`,
restart `hevd`, `hev skills sync`, `hev update --channel edge`), as on
[Updating](https://hevkit.com/docs/updating#moving-a-cluster-to-edge-the-first-time).
From then on the commands above keep it there.

**Rollback.** `hev dispatch hosts update --host NAME --to X.Y.Z-edge.<sha>`
moves a host to a named earlier build (the newest 10 per train stay
published); on the host itself `hev update --rollback` returns to the build
the last update replaced, which is always kept. Restart the dashboard with
the previous image if it moved.

**How a host reports itself.** `hev version` prints `X.Y.Z-edge.<sha>`.
`hev dispatch hosts` lists every host's version, marks one that differs from
the sender's "(version mismatch)", and shows each host's channel and whether
it is behind it ("yes, N commits" on edge). `~/.hev/update/state.json` on a
host records what its updater did.

**Compatibility one version back.** Hosts move to a new build at different
moments, so any build must work with the build before it (and the first edge
of a train with the last release): the records it stores and reads (dispatch
state, the queue, notices, config, the trace archive and the board) and the
dispatch RPC, in both directions. A change that cannot is split, or lands a
reader one build before the writer. Reviewers hold feature PRs to this.

**Operator's choice, not the users' default.** Edge is how the operator runs
their own cluster. Users install releases: Homebrew, the release archives,
`go install github.com/hev/kit/cmd/hev@vX.Y.Z`, or `@main`, which is the
latest release. `hev update` follows `release` unless a host opts in.

## Setup the train needs

| Thing | Where | Status |
|---|---|---|
| Train branch `v0.7` | branch on `hev/kit` from `main` (a7075f0) | exists; open train PRs retargeted to it |
| `KIT_RELEASE_DRAFT=on` | repository variable | FOH sets it to start keeping the draft; until then every run is a dry run |
| `KIT_DOCS_PREVIEW_URL` | repository secret | optional; the preview's private address, for links in the draft |
| `KIT_ADVANCE_MAIN=on`, `KIT_MAIN_PUSH_TOKEN` | repository variable; secret | optional; without them FOH runs `release-advance.py --apply` by hand after verify |
| `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, `HOMEBREW_TAP_TOKEN` | repository secrets | exist (used by every release so far) |
| `KIT_SITE_PAGES_PROJECT`; `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` | variable; secrets | optional; without them production is deployed by hand from the `site.yml` artifact |
| The train docs branch | `train/vX.Y-docs` + draft PR against `vX.Y` (0.7: `d/kit-docs-07-20261111`, #210) | exists for 0.7 |
| The preview on the mini | launchd agents: `docs-preview.sh serve` (KeepAlive) and `refresh` (every 300 s), port 4370, following the train docs branch | running for 0.7 |
| `edge.yml` | workflow on `vX.Y` pushes; `GITHUB_TOKEN` with `contents: write`, `id-token: write`, `attestations: write` | no secret or setting; publishes from the first push after it merges to the train |
| Dependabot `target-branch` | `.github/dependabot.yml`, set in the `X.Y.0` prep PR | at 0.7's cut: `v0.8` |

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
