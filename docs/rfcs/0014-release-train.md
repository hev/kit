# RFC 0014 — Release train: a train branch, docs first, a draft release as the ledger, publishing as the gate

**State: Proposed** (2026-11-12). Operational detail:
[`docs/runbooks/release-train.md`](../runbooks/release-train.md). 0.7 is the
first release on it.

## Summary

Kit releases become a train modelled on Layer's (Layer RFCs 0102 and 0103,
its gateway release runbook and its UAT and delivery runbook), cut down to what
a single public repository needs:

- **A train branch per release, Layer's branch scheme.** Each train has a
  semver branch `vX.Y`, opened from `main` when the train opens. Feature PRs
  for the train merge into `vX.Y`. **`main` does not move between releases**:
  it is the latest published release, so `go install …@main`, the README and
  hevkit.com's pin all describe released behaviour. After a release ships,
  `main` is fast-forwarded to its tag (never forced) and the next train branch
  opens. The same `vX.Y` branch carries the `X.Y.Z` patches.
- **Docs first.** A train's features are documented before they are built,
  on a **train docs branch**: a draft PR against `vX.Y` that carries the docs
  for every planned feature, marked "Coming in X.Y", and never merges. As each
  feature merges into `vX.Y`, its pages move into the feature's PR, so the
  train branch's docs describe only what it does.
- **A docs preview of the train**, built and served on the private network by
  the mini (`scripts/docs-preview.sh`, kept running by launchd). While the
  train is open it follows the train docs branch (`vX.Y` plus the planned
  features' docs); at the cut, the train branch. Production hevkit.com stays
  on the latest published release.
- **One draft GitHub release per train**, re-rendered on every merge to `vX.Y`
  from `docs/releases/vX.Y.Z.md` and the merge log, targeting the train branch
  commit it was rendered from (`scripts/release-draft.py`, `release-draft.yml`).
  It is the ledger.
- **Publishing the draft is the gate, and Adam's one act.** It creates the tag
  on the train branch and starts the existing build (`release.yml`, now on
  `release: published`), which then fast-forwards `main`.
- A **preflight** before publish (including that `main` is an ancestor of the
  train branch), a **verify** after (`scripts/release-check.py`), and
  **advance** (`scripts/release-advance.py`): `main` to the tag, the next train
  branch opened.
- **Edge across the operator's own cluster.** Adam's machines run a build of
  the open train branch's tip, not the last release, so every merged feature
  is exercised on real work before the draft is published. A release is a
  promotion of what has already been running. Users install releases.

## Motivation

Today a release is a tag pushed after a short checklist (`docs/release.md`).
What is missing, from what Adam asked for:

1. **Docs are not reviewable before a release.** The site lived on a branch
   (`site/kit-0.5`, PR #129) deployed by hand, so a feature's docs could not
   land with the feature and there was nothing to read before shipping.
2. **Nothing accumulates "what is in the next release".** Release notes were
   `--generate-notes` at tag time; the only way to see what 0.7 holds was to
   walk the log.
3. **The release decision is a git operation.** Pushing a tag is easy to do
   by accident, by an agent, and without reading anything first.
4. **`main` mixes released and unreleased work.** Anyone who installs from
   `main`, reads the README or follows hevkit.com's install line gets whatever
   merged last, not a release.

## Design

### The train branch

Layer's scheme (RFC 0102: "Public `main` never moves between releases"),
in one repository:

- **Open.** When Adam confirms a train's manifest, FOH creates `vX.Y` from
  `main` (which is the latest release). For the train after a release,
  `release-advance.py` opens it automatically at the release tag.
- **During.** Every feature PR for the train targets `vX.Y`; dispatch orders
  for it use `base: vX.Y`. CI (`ci`, `adapters`, `site`) runs on pushes to and
  PRs into `vX.Y` exactly as for `main`. `main` takes no merges.
- **Ship.** The draft targets the train branch commit it was rendered from;
  publishing tags that commit. `release.yml` builds, ships and verifies, then
  its `advance` job fast-forwards `main` to the tag and opens `vX.(Y+1)` from
  the tag. A plain push, never forced: if `main` is not an ancestor of the tag
  the job stops and FOH reports to Adam.
- **After.** `vX.Y` stays as the `X.Y` patch line (below). `main` equals the
  tag.

**Lineage.** Fast-forwarding needs `main` to be an ancestor of the tag. It
holds by construction (the train branch is opened from `main`, and `main`
moves only to a tag on that line), except after a patch on the newest line
moves `main` while the next train is open. Then FOH merges `vX.Y` into
`vX.(Y+1)` through a PR (a merge, not a rebase), which restores it without
forcing anything. `release-advance.py` warns when this is needed, the draft
workflow says so in its summary, and the preflight fails until it is done.
Layer's mirror re-parents the train branch with a force push; kit has no
mirror and does not need to.

**Fast-forwarding `main` under its protection.** `main` requires the `go`
check (enforced for non-admins). The tag commit passed `go` when it was pushed
to the train branch (CI runs on `vX.Y` pushes for this reason), so the check
is met by a plain push of that commit. The token is the constraint:
`GITHUB_TOKEN` has no `workflows` permission, and GitHub refuses a ref update
from it that brings in changes under `.github/workflows/`, which most
releases do. So the `advance` job uses a `KIT_MAIN_PUSH_TOKEN` secret: a
fine-grained token on `hev/kit` with Contents and Workflows read and write,
owned by someone allowed to push to `main`. Until it is set, and until the
repository variable `KIT_ADVANCE_MAIN=on`, the job is a dry run that prints
the pushes for FOH to run by hand (as a maintainer: `git push origin
vX.Y.Z^{commit}:refs/heads/main`, no `--force`). Branch protection is not
changed here.

**Patch releases: one scheme.** The old idea of a `release/vX.Y` branch cut
from a tag is retired; `vX.Y` *is* the patch line. A fix for a published
`X.Y` merges into `vX.Y` (and into the open train, `vX.(Y+1)`, if it applies
there). The draft workflow then drafts `vX.Y.(Z+1)` from `vX.Y`. On the newest
line, `advance` fast-forwards `main` to the patch tag, and FOH merges `vX.Y`
into the open train. On an older line (say `v0.6` after 0.7.0 shipped), the
branch is created from its last tag when first needed, and `main` never moves
for it. Layer's break-glass `release/vX.Y` exists because its train branch is
a generated mirror on another repository; kit's train branch is its own.

**Dependabot.** It reads `.github/dependabot.yml` from the default branch
(`main`) only, and `main` moves only at a release. So each `X.Y.0` carries the
config for the train that opens after it: every update has
`target-branch: vX.(Y+1)`, set in the cut's prep PR and checked by the
preflight. Dependabot security updates always target the default branch; FOH
does not merge them into `main`, but applies the bump on the train branch (or
on the patch line, then ships a patch). During 0.7 `main`'s config still
targets `main`, so Dependabot's PRs wait (or are re-done on `v0.7` by hand)
until 0.7.0 ships the new config.

**Dispatch orders.** During a train, feature orders use `base: vX.Y`; a fix
for a published line uses `base: vX.Y` of that line; a docs worker for planned
features uses the train docs branch. Orders do not use `base: main`, except
for work that is not on any train (none, normally).

**The default branch stays `main`.** It is what people clone and read. PRs
default to it, so FOH retargets a PR opened against `main` by mistake.

**0.7, honestly.** `main` is already ahead of v0.6.2: the merges since that
tag, plus the site (#129) and this RFC (#211), landed on `main` before `v0.7`
was branched from it (at a7075f0). So during this one train `main` is not
exactly the last release; it becomes so when 0.7.0 ships and `main`
fast-forwards to its tag. `main` is not rewound to v0.6.2.

### Docs first

From Layer's UAT and delivery runbook: for substantial features, the
customer-facing docs and examples are written and agreed with Adam before
complex implementation, proportionate to uncertainty; docs and code land
together; draft docs stay on the feature branch until the behaviour ships.

Adam's requirement is stronger than "with the code": he reads a train's
features in the preview **before** they are built or merged. Kept on each
feature's branch, those docs would be invisible until the feature merged. So
each train has one **train docs branch**:

- **What it is.** A branch off `vX.Y` with a draft PR against `vX.Y`,
  holding the docs for every feature planned for the train (new pages,
  sections of existing pages, `docsNav` entries). Unreleased content carries a
  visible "Coming in X.Y" marker (the site's `<Callout label="Coming in
  X.Y">`, or **Coming in X.Y** in a table row). It is never merged.
- **Name.** `train/vX.Y-docs` (e.g. `train/v0.8-docs`), PR titled
  "vX.Y train docs (do not merge)". 0.7's was opened before this convention as
  `d/kit-docs-07-20261111` (#210, retargeted to `v0.7`); its notes file names
  it with a `<!-- train-docs: BRANCH -->` line, which overrides the
  convention.
- **Written first.** A feature's docs are written there and agreed with Adam
  before complex implementation (Layer's rule, proportionate: a small fix
  needs only an updated example). The implementing worker reads them as the
  spec.
- **Moves to the train branch with the feature.** The feature's PR carries
  its pages over from the train docs branch with the "Coming in X.Y" markers
  removed (or cherry-picks the branch's commits for that feature). After the
  merge, FOH rebases the train docs branch on `vX.Y` and drops what is now
  there. A merged feature without docs is a bug. So `vX.Y`'s docs describe
  only what `vX.Y` does, and the preflight refuses a release whose docs still
  say "Coming in".
- **Closed at the cut.** What remains is either moved to the next train's
  docs branch (the feature waits) or dropped. FOH closes the PR and deletes the
  branch; the preview then follows `vX.Y`.

Docs for unbuilt features are kept from production by never being on a train
branch: production shows only released tags, so no flags or hidden pages are
needed.

The runbook also records a standing docs requirement from 0.7 on: the docs
state the free/paid seam (free kit keeps everything on the file system; paid
adds hev layer, which moves it off the file system). It is a docs requirement,
not a release highlight.

### The docs preview

The site built from one ref and served from the mini over the tailnet on a
fixed port (4370 by default, the port 0.7's preview uses).
`docs-preview.sh build REF` builds into a per-commit directory and swaps a
`current` symlink only on success; `serve` serves `current` with Python's
static server; `refresh`, run every five minutes, fetches and rebuilds only
when the followed ref moved (`KIT_PREVIEW_REF`). On the mini it is kept
running by two launchd agents: `serve` with KeepAlive, and `refresh` on a
300-second interval.

What the preview follows, by stage:

| Stage | `KIT_PREVIEW_REF` | Shows |
|---|---|---|
| Train open | `origin/<train docs branch>` (fallback `origin/vX.Y`) | `vX.Y`'s docs plus the planned features' docs, marked "Coming in X.Y" |
| A feature merges into `vX.Y` | same; FOH rebases the docs branch on `vX.Y` | the feature's pages now come from `vX.Y`, unmarked |
| Cut (docs branch closed) | `origin/vX.Y` (refresh falls back to `KIT_PREVIEW_FALLBACK` on its own when the branch is deleted) | exactly what the release documents |
| Next train opens | the next train docs branch, fallback `origin/vX.(Y+1)` | the next train |

A patch line's docs are rarely worth a preview; when one is, a second copy
follows `origin/vX.Y` on its own port.

The server answers on the mini's MagicDNS name because Python's `http.server`
has no host check; `scripts/test-docs-preview.py` asserts a 200 for a
MagicDNS Host header.

No Cloudflare, no public hostname, no secret: the preview is for Adam and the
team, all on the tailnet. Its address is private, so the draft links it only
when the `KIT_DOCS_PREVIEW_URL` secret is set, and the published release body
replaces that section with hevkit.com links. Because the preview follows the
train docs branch, which is `vX.Y` plus the planned docs, every page the
draft lists resolves there: the draft lists the pages changed on `vX.Y` since
the last tag and, separately, the pages on the train docs branch that are not
on `vX.Y` yet. A push to a `train/vX.Y-docs` branch re-renders `vX.Y`'s draft
so the second list stays current.

### Production stays on the latest release

`site.yml` rebuilds production on `release: published` from the latest
published release's tag: `site/scripts/pin-docs-release.sh <tag>` pins
`site/src/content/docs` to it and writes `/docs-release.json` naming it. The
Cloudflare Pages deploy runs only when its project variable and secrets
exist; until then the pinned build is a workflow artifact for the hand deploy
used today. Unreleased work is on train branches, never on `main` or a tag,
so it cannot reach production.

### The draft release

`release-draft.py --branch vX.Y` renders a body from one commit of the train
branch: the notes file, one line per merged PR since the last tag grouped by
kind, the docs pages changed since that tag, and a `version/base/source`
marker. The version is always `X.Y.Z`: the lowest untagged notes file on that
line, else `X.Y.0`, else (once `X.Y.0` has shipped) the next patch. It is
pure (no clock, no GitHub), so re-rendering is idempotent. With `--apply` it
upserts the draft through `gh`: it always sends `tag_name` and
`target_commitish` (the rendered commit), never touches a published release,
refuses two drafts for one version, restores an `untagged-…` draft, and
writes nothing for a patch line with nothing new on it. `release-draft.yml`
runs it on every push to a `vX.Y` branch (and to `train/vX.Y-docs`); it
writes to GitHub only when `KIT_RELEASE_DRAFT=on`.

The target is the exact commit the body was rendered from, so the tag Adam's
publish creates points at what he read. (Layer targets the branch name; a
commit gives the same fencing and cannot drift between reading and
publishing.)

### The gate: publishing starts the build

**Does a tag created by publishing a release trigger `push: tags`?** GitHub
does not document that it does. The documented event for this act is
`release` with activity `published` ("A release, pre-release, or draft of a
release was published", [webhook events: release][wh-release]); `push`'s docs
say only that tag events are not created when more than three tags are pushed
at once ([events that trigger workflows][ev]), and community reports of
release-created tags firing `push` are mixed. The other constraint is the
token rule: "events triggered by the `GITHUB_TOKEN` … will not create a new
workflow run", except `workflow_dispatch` and `repository_dispatch`
([triggering a workflow][trig]). The draft is written by `GITHUB_TOKEN`, but
**publishing is done by Adam**, so the `published` event is his and starts
workflows. (Drafts themselves do not: "Workflows are not triggered for the
`created`, `edited`, or `deleted` activity types for draft releases", [ev].)

So `release.yml` runs on `release: types: [published]` (plus
`workflow_dispatch` with a `tag` input to re-run a published release). No
credential changes for the build: the existing `DOCKERHUB_*` and
`HOMEBREW_TAP_TOKEN` secrets serve as before, and `GITHUB_TOKEN` uploads the
assets. The workflow uploads to the existing release instead of `gh release
create`, refuses drafts and tagless runs, and only moves `:latest`, the tap
and `main` for the newest release. This has not run end to end (no release
was published for this RFC); the first publish is the proof, and
`workflow_dispatch -f tag=vX.Y.Z` is the fallback if it ever does not fire.

### Preflight, verify, advance

`release-check.py preflight` (and `release-preflight.yml`) checks what the old
checklist asked for and what kit's build depends on: the commit is on `vX.Y`
and `main` is its ancestor, notes final, `go vet` (also `GOOS=windows`),
`go test`, all five archives build with the version injected and report it,
bundled skills install current in a temporary HOME, the binary pins
`hevlayer/kit:X.Y.Z` and that image is not yet pushed (and builds, with
`--docker`), the changelog and install pages name the version, no "Coming in"
remains and the train docs branch is closed, Dependabot targets the next
train (for `X.Y.0`), and the draft targets this commit. Browser validation and
the bundled-skills content review stay manual. `verify` checks assets,
checksums, a fresh download's `hev version`, the Docker Hub tags, the tap
formulas and the docs pin. Both are read-only. `release-advance.py` then
fast-forwards `main` and opens the next train (above).

### Edge across the operator's cluster

Adam: "my cluster runs edge across everything (including that dashboard
there)." **Edge** is the tip of the open train branch `vX.Y`. The operator's
own machines run a build of it, not the last release, so each merged feature
does real work before the draft is published, and publishing promotes what
has already been running.

- **What "everything" is.** Every host that sends or receives dispatch: the
  `hev` on `PATH` for senders and for targets (and the `hev` that supervisors
  and plugins exec), the capture and dispatch daemon (`hevd`) on each host,
  the kit dashboard service (`hev up`'s image), and the bundled skills, which
  `hev skills sync` refreshes from the edge binary into each harness.
- **Moving a host to a new edge build without disturbing live runs.** The new
  binary is installed beside the current one (a versioned file), then the
  name on `PATH` is switched to it atomically (a rename over a symlink).
  Running supervisors keep the binary they started with (an exec'd process
  holds its inode), so in-flight runs finish on the old build; new runs start
  on the new one. The daemon restarts through its service manager, which
  drains or hands over as it does for a release upgrade today. The dashboard
  is rebuilt from the same commit and replaced the same way.
- **How it reports itself.** An edge build is stamped `X.Y.0-edge.<sha>`
  (`-X …/internal/version.Version=0.7.0-edge.1a2b3c4`), so `hev version`
  names the commit. `hev dispatch hosts` already prints each host's version
  and marks one that differs from the sender's with "(version mismatch)", so
  a host behind edge is visible from any sender.
- **Rollback.** The previous build stays installed beside the new one;
  switching the link back (and restarting the daemon and dashboard) is the
  rollback. Keep at least the last two.
- **Compatibility one version back.** Hosts update at different moments, so
  for any two adjacent edge builds (and the last release against the first
  edge of a train) stored records (dispatch state, notices, queue entries,
  config, the trace archive and board) and the dispatch RPC must read and
  answer each other. A change that cannot is split, or ships a reader first.
  Reviewers hold feature PRs to this.
- **Operator's choice.** Edge is how the operator runs their own cluster. It
  is not the default for users: they install releases (`brew`, the release
  archives, `go install …@vX.Y.Z` or `@main`, which is the latest release).
  Nothing in kit points a user at edge.

The **updater** that moves a host to edge is follow-up work, not built here.
Its contract: build (or fetch) the edge binary for the host's OS and arch from
a named train-branch commit, stamped `X.Y.0-edge.<sha>`; install beside the
current build and switch atomically; never signal or replace a running
supervisor; restart the daemon through its service manager; rebuild and
replace the dashboard from the same commit (its image tag is that version, so
`hev up` finds it locally); run `hev skills sync`; keep the previous build and
offer a one-command rollback; report what it did, and refuse to move a host
to a commit older than the one it runs. One more piece of follow-up: `hev
dispatch hosts` could say "behind edge by N" rather than only "version
mismatch".

### Roles

Workers stop at a commit; a docs worker writes a train's planned docs on the
train docs branch, a feature worker (with `base: vX.Y`) moves its pages into
its feature PR. FOH pushes, merges into `vX.Y`, opens the train branch and the
train docs branch, rebases the docs branch after each feature merge, closes it
at the cut, points the preview at each stage's ref, keeps the notes and the
draft current, runs the preflight and the preview, merges `vX.Y` into the next
train after a patch, runs the `advance` pushes by hand until they are
automated, and reports. Only Adam publishes.

## Where kit departs from Layer, and why

| Layer | Kit | Why |
|---|---|---|
| Private source repo mirrored to a public repo; the semver branch `vX.Y` is on the public side and generated | One public repo; `vX.Y` is an ordinary branch features merge into | Kit has no private source and no mirror. The branch scheme is the same; the machinery that writes it is not needed. |
| The draft targets the branch `vX.Y` | The draft targets the exact `vX.Y` commit it was rendered from | Same fencing as Layer's `source=` marker, and the tag is exactly what Adam read even if `vX.Y` moved since. |
| Lineage repaired by the mirror force-pushing a re-parented `vX.Y` | FOH merges `vX.Y` into the open train through a PR; nothing is forced | Kit's train branch has human merges on it; a merge keeps them and needs no force-push token. |
| Break-glass patches on `release/vX.Y` cut from the tag | Patches on `vX.Y` itself | One scheme: kit's `vX.Y` is real source, so it can carry patches; a second branch name would be a second scheme. |
| Publish → relay workflow on the public repo → `repository_dispatch` to the private repo (needs `LAYER_PRO_DISPATCH_TOKEN`) → train; a watcher re-dispatches | `release: published` starts `release.yml` in the same repo | Same repo, so the event is direct. No relay token, no watcher. |
| `release-prep` bump PR (`X.Y.Z-dev` → `X.Y.Z` in Cargo and client manifests), CE image pins, auto-advance to the next `-dev` | No bump. The tag is the version (ldflags); the image pin is derived from it | Kit has no version in its manifests. The prep PR updates the docs pins and Dependabot's target; the preflight checks them. |
| Images, npm, PyPI and Go module released in lockstep; license-gate smokes; client mirrors | One binary, one image, one tap | Kit ships no clients and no licensed images. |
| Versioned Cloudflare preview at `vX.Y.x.hevlayer.com` deployed by CI | Preview served from the mini on the tailnet, kept up by launchd | Adam's decision (2026-11-12): no Cloudflare and no public hostname for the preview. The audience is all on the tailnet. |
| The preview tracks `main`; draft docs stay on feature branches until the behaviour ships | The preview follows a train docs branch (`vX.Y` plus the planned features' docs) until the cut, then `vX.Y` | Adam reads a train's features before they are built. |
| Production docs pinned at deploy by `pin-docs-release.mjs` (Node, store manifests, digest) | `pin-docs-release.sh`: restore `content/docs` from the tag | Kit's docs have no per-edition store manifest or search digest. |
| The cluster runs `main` | The operator's cluster runs edge, the open `vX.Y`'s tip | Kit's `main` is the last release, so edge is the train branch. |
| Release captain agent with status and Slack notify tooling; weekly credential preflight | FOH runs the runbook | Kit's train has a handful of steps and a few secrets; a captain is machinery for a problem kit doesn't have yet. |
| Factory self-merge policy changes (RFC 0103) | Unchanged | Out of scope; FOH already merges kit PRs. |

## Migration

1. #211 (the first version of this RFC) merged to `main`. FOH created `v0.7`
   from it (a7075f0) and retargeted the open train PRs to `v0.7`. This
   change merges into `v0.7`.
2. FOH sets `KIT_RELEASE_DRAFT=on`; the 0.7.0 draft appears and follows
   `v0.7`. FOH runs the draft workflow once by hand for `v0.7` to start it.
3. The 0.7 preview runs on the mini on port 4370 under launchd, following 0.7's
   train docs branch, with `KIT_PREVIEW_FALLBACK=origin/v0.7`.
4. 0.7 rides: features land in `v0.7` with docs, notes accumulate, the cut
   follows the runbook (including Dependabot's `target-branch: v0.8`), Adam
   publishes. The first publish proves the trigger; `advance` (or FOH by hand)
   fast-forwards `main` to v0.7.0 and opens `v0.8`.
5. Adam's cluster moves to edge builds of `v0.7` when an updater exists; until
   then, by hand.

## Open questions

- Production deploy: keep the hand deploy (from the `site.yml` artifact) or
  give CI the Pages project and token?
- Cut criteria for 0.7: the manifest is confirmed (headliners plus green
  candidates); a date or a "both headliners merged" rule is Adam's call.
- `KIT_MAIN_PUSH_TOKEN`: whose token, or keep the fast-forward a manual FOH
  step.

[wh-release]: https://docs.github.com/en/webhooks/webhook-events-and-payloads#release
[ev]: https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/events-that-trigger-workflows#release
[trig]: https://docs.github.com/en/actions/using-workflows/triggering-a-workflow#triggering-a-workflow-from-a-workflow
