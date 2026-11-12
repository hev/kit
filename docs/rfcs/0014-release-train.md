# RFC 0014 — Release train: docs first, a draft release as the ledger, publishing as the gate

**State: Proposed** (2026-11-12). Operational detail:
[`docs/runbooks/release-train.md`](../runbooks/release-train.md). 0.7 is the
first release on it.

## Summary

Kit releases become a train modelled on Layer's (Layer RFCs 0102 and 0103,
its gateway release runbook and its UAT and delivery runbook), cut down to what
a single public repository needs:

- **Docs first.** A feature's docs are written and readable in a docs preview
  before or with its implementation, and land on `main` in the feature's PR.
  The site moves onto `main` to make that possible (done: #129).
- **A docs preview of `main`**, built and served on the private network by the
  mini (`scripts/docs-preview.sh`). Production hevkit.com stays pinned to the
  latest published release.
- **One draft GitHub release per next version** on `hev/kit`, re-rendered on
  every merge from `docs/releases/vX.Y.Z.md` and the merge log
  (`scripts/release-draft.py`, `release-draft.yml`). It is the ledger.
- **Publishing the draft is the gate, and Adam's one act.** It creates the tag
  and starts the existing build (`release.yml`, now on `release: published`).
- A **preflight** before publish and a **verify** after
  (`scripts/release-check.py`), and a defined **patch** path.

## Motivation

Today a release is a tag pushed after a short checklist (`docs/release.md`).
Three things are missing that Adam asked for:

1. **Docs are not reviewable before a release.** The site lived on a branch
   (`site/kit-0.5`, PR #129) deployed by hand, so a feature's docs could not
   land with the feature and there was nothing to read before shipping.
2. **Nothing accumulates "what is in the next release".** Release notes were
   `--generate-notes` at tag time; the only way to see what 0.7 holds was to
   walk the log.
3. **The release decision is a git operation.** Pushing a tag is easy to do
   by accident, by an agent, and without reading anything first.

## Design

### Docs first

From Layer's UAT and delivery runbook: for substantial features, the
customer-facing docs and examples are written and agreed with Adam before
complex implementation, proportionate to uncertainty; docs and code land
together; draft docs stay on the feature branch until the behaviour ships.

For kit that means: the page lives in `site/src/content/docs/`; it merges in
the feature's PR (built by `site.yml` on the PR); a merged feature without docs
is a bug; and docs for unbuilt features are kept from production simply by not
being on `main`. Production shows only released tags, so no flags or hidden
pages are needed.

The runbook also records a standing docs requirement from 0.7 on: the docs
state the free/paid seam (free kit keeps everything on the file system; paid
adds hev layer, which moves it off the file system). It is a docs requirement,
not a release highlight.

### The docs preview

The site built from `main`, served from the mini over the tailnet on a fixed
port (4342, which the hand-run preview already uses). `docs-preview.sh build
REF` builds into a per-commit directory and swaps a `current` symlink only on
success; `serve` serves `current` with Python's static server; `refresh`,
run every five minutes, fetches and rebuilds only when `origin/main` moved.
A patch train can run a second copy on `release/vX.Y`.

No Cloudflare, no public hostname, no secret: the preview is for Adam and the
team, all on the tailnet. Its address is private, so the draft links it only
when the `KIT_DOCS_PREVIEW_URL` secret is set, and the published release body
replaces that section with hevkit.com links.

### Production stays on the latest release

`site.yml` rebuilds production on `release: published`:
`site/scripts/pin-docs-release.sh <latest tag>` restores
`site/src/content/docs` from the tag, the rest of the site comes from `main`,
and `/docs-release.json` names the tag. The Cloudflare Pages deploy runs only
when its project variable and secrets exist; until then the pinned build is a
workflow artifact for the hand deploy used today.

### The draft release

`release-draft.py` renders a body from one commit: the notes file, one line per
merged PR since the last tag grouped by kind, the docs pages changed since that
tag, and a `version/base/source` marker. It is pure (no clock, no GitHub), so
re-rendering is idempotent. With `--apply` it upserts the draft through `gh`:
it always sends `tag_name` and `target_commitish` (the rendered commit), never
touches a published release, refuses two drafts for one version, and restores
an `untagged-…` draft. `release-draft.yml` runs it on every push to `main` and
`release/v*`; it writes to GitHub only when `KIT_RELEASE_DRAFT=on`.

The target is the exact commit the body was rendered from, so the tag Adam's
publish creates points at what he read.

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

So `release.yml` moves from `push: tags: v*` to `release: types: [published]`
(plus `workflow_dispatch` with a `tag` input to re-run a published release).
No credential changes: the existing `DOCKERHUB_*` and `HOMEBREW_TAP_TOKEN`
secrets serve as before, and `GITHUB_TOKEN` uploads the assets. Because the
release now exists before the run, the workflow uploads to it instead of
`gh release create`, refuses drafts and tagless runs, and only moves
`:latest` and the tap for the newest release. This has not run end to end
(no release was published for this RFC); the first publish is the proof, and
`workflow_dispatch -f tag=vX.Y.Z` is the fallback if it ever does not fire.

### Preflight and verify

`release-check.py preflight` (and `release-preflight.yml`) checks what the old
checklist asked for and what kit's build depends on: notes final, `go vet`
(also `GOOS=windows`), `go test`, all five archives build with the version
injected and report it, bundled skills install current in a temporary HOME,
the binary pins `hevlayer/kit:X.Y.Z` and that image is not yet pushed (and
builds, with `--docker`), the changelog and install pages name the version, and
the draft targets this commit. Browser validation and the bundled-skills
content review stay manual. `verify` checks assets, checksums, a fresh
download's `hev version`, the Docker Hub tags, the tap formulas and the docs
pin. Both are read-only.

### Roles

Workers stop at a commit. FOH pushes, merges, keeps the notes and the draft
current, runs the preflight and the preview, and reports. Only Adam publishes.

## Where kit departs from Layer, and why

| Layer | Kit | Why |
|---|---|---|
| Private source repo mirrored to a public repo; semver branch `vX.Y` on the public side; the draft targets it | One public repo; the draft targets the exact `main` commit it was rendered from | Kit has no private source and no mirror. A commit SHA gives the same exact-source fencing as Layer's `source=` marker without a branch to maintain. |
| Publish → relay workflow on the public repo → `repository_dispatch` to the private repo (needs `LAYER_PRO_DISPATCH_TOKEN`) → train; a watcher re-dispatches | `release: published` starts `release.yml` in the same repo | Same repo, so the event is direct. No relay token, no watcher. |
| `release-prep` bump PR (`X.Y.Z-dev` → `X.Y.Z` in Cargo and client manifests), CE image pins, auto-advance to the next `-dev` | No bump. The tag is the version (ldflags); the image pin is derived from it | Kit has no version in its manifests. The only pins are the docs pages, which the prep PR updates and the preflight checks. |
| Images, npm, PyPI and Go module released in lockstep; license-gate smokes; client mirrors | One binary, one image, one tap | Kit ships no clients and no licensed images. |
| Versioned Cloudflare preview at `vX.Y.x.hevlayer.com` deployed by CI | Preview served from the mini on the tailnet | Adam's decision (2026-11-12): no Cloudflare and no public hostname for the preview. The audience is all on the tailnet. |
| Production docs pinned at deploy by `pin-docs-release.mjs` (Node, store manifests, digest) | `pin-docs-release.sh`: restore `content/docs` from the tag | Kit's docs have no per-edition store manifest or search digest. |
| Release captain agent with status and Slack notify tooling; weekly credential preflight | FOH runs the runbook | Kit's train has a handful of steps and three secrets; a captain is machinery for a problem kit doesn't have yet. |
| Factory self-merge policy changes (RFC 0103) | Unchanged | Out of scope; FOH already merges kit PRs. |

## Migration

1. Merge this RFC's PR: workflows land dry (`KIT_RELEASE_DRAFT` unset), the
   release workflow switches to `release: published`.
2. FOH sets `KIT_RELEASE_DRAFT=on`; the 0.7.0 draft appears and follows `main`.
3. FOH starts the preview on the mini in place of the hand-run one.
4. 0.7 rides: features land with docs, notes accumulate, the cut follows the
   runbook, Adam publishes. The first publish proves the trigger.

## Open questions

- Production deploy: keep the hand deploy (from the `site.yml` artifact) or
  give CI the Pages project and token?
- Cut criteria for 0.7: the manifest is confirmed (headliners plus green
  candidates); a date or a "both headliners merged" rule is Adam's call.

[wh-release]: https://docs.github.com/en/webhooks/webhook-events-and-payloads#release
[ev]: https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/events-that-trigger-workflows#release
[trig]: https://docs.github.com/en/actions/using-workflows/triggering-a-workflow#triggering-a-workflow-from-a-workflow
