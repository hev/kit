# RFC 0018: The skills shelf — a few pinned popular packs, enabled per run, searchable in Layer

Status: Implemented (2026-11-16) in the same PR. Decision: board memo #500
and its replies. Builds on [RFC 0012](0012-skill-sources.md) (skill sources
with commit pinning) and [skill capture](../skill-capture.md). Operator guide:
[docs/skills-shelf.md](../skills-shelf.md).

> Numbering: the order for this work asked for RFC 0014, but 0014 is the
> release train and 0015 to 0017 are taken on open branches (host jobs,
> cluster, notify), so this is 0018.

## Summary

kit welcomes people's own and popular skills through a curated **shelf**, not
a marketplace. A shelf pack is an RFC 0012 skill source with a name, a kind
and an exact pinned commit. `hev skills add OWNER/REPO[//SUBDIR][@REF]` writes
the entry; `hev skills shelf` lists each pack's pin, licence, author and
skills. Packs are installed to the pinned store but are **not active**: no
harness sees them until a run enables named skills with `skills: [pack/skill,
...]` in its order or context recipe, and dispatch links exactly those into
that run's worktree for as long as the harness runs. A pin moves only by
`hev skills shelf bump PACK --to SHA` after someone reviews `hev skills shelf
diff PACK`. Each pinned snapshot is indexed as the skills collection in the
operator's Layer namespace, and skill capture records the pinned commit for
loads that resolve into a snapshot.

## Motivation

- People already reach for popular packs (`npx skills add ...`). Copying them
  into `~/.claude/skills` makes every session load them, with no record of
  which version ran and no way to compare runs with and without them.
- Kit sees outcomes. If packs are enabled per run and every load carries its
  commit, kit can run the same orders with and without a pack and report the
  review verdict, rework count, tokens and wall time. That is evidence nobody
  else has; it must be framed as "we ran the popular packs", never as an
  endorsement or partnership.
- An upstream pack is someone else's instructions running with the worker's
  permissions. Pinning to a reviewed commit, never auto-updating, and
  reviewing every diff is the minimum safe posture (injected instructions,
  exfiltration, scope creep).

## Decision

1. **Shelf, not store.** A pack is referenced and pinned, never vendored into
   kit. Attribution (publisher, homepage) and licence (SPDX id from the
   nearest licence file) are shown wherever kit shows a shelf skill: `hev
   skills shelf`, `hev skills search` hits, every indexed chunk and the run
   record.
2. **Kinds.** `kind = "mode" | "tool" | "domain"` per pack. A mode owns the
   workflow (pstack's poteto-mode, obra/superpowers), so a run enables skills
   from at most one mode pack; tools and domain skills compose. The kind is
   the operator's classification in config.
3. **Installed is not active.** `hev skills sync` installs shelf packs to
   `~/.local/opt/hev-skills-SOURCE-COMMIT` (only the pack's subdirectory and
   the licence files on its path) and links nothing. Bundled kit skills and
   the operator's own skills and team sources are unchanged.
4. **Per-run enablement.** The target resolves the run's skills (order plus
   recipe) against its own shelf when the run is accepted: an unknown pack or
   skill, a pin that is configured but not installed, two packs giving a skill
   the same name, or more than one mode pack refuses the run with a clear
   error. At launch dispatch links exactly those skills into the worktree's
   project skill root for the run's harness, keeps them out of git and
   removes them when the harness exits.
5. **Pin, review, bump.** `diff` shows upstream changes since the pin; `bump`
   moves the pin only to a commit given with `--to` that is on the pack's
   ref, then reinstalls and re-indexes. There is no scheduler and no
   auto-update.
6. **Layer index.** Each installed pin is chunked and written to
   `<namespace>-skills`, keyed by (source, commit, path, part), with licence
   and attribution on every row. Search scopes to the commits installed now.
   Operator namespace only; republishing through hosted Layer needs a
   separate licence decision.
7. **Capture.** A skill load whose path is in a pinned snapshot, directly or
   through a run's live link, records that commit in `skill_loads[].commit`.

## Goals

- Adam can put the seven starting packs on a host's shelf, enable a mode and
  a tool for one run, and see from the run's transcript that the worker saw
  exactly those skills.
- Every shelf skill a run loads can be joined to the commit it came from.
- No pack changes what an ordinary run, or any session outside dispatch,
  sees.

## Non-goals

- A marketplace, ratings, search over packs nobody pinned, or installing from
  anything but a Git source.
- Auto-update, a scheduler or a daemon. Review automation is documented
  (below), not built.
- Enabling shelf skills for interactive sessions. `skills:` is a dispatch
  feature; an operator who wants a pack everywhere can still add it as a
  plain RFC 0012 team source.
- MCP servers a pack describes (RFC 0012's MCP gap still applies).
- The A/B evidence report. This RFC makes it possible (per-run enablement and
  commit-joined loads); the comparison is a follow-up order.

## Vocabulary

- **Shelf**: the shelf packs configured on a host.
- **Pack**: one `[[skills.sources]]` entry with `name`, `kind` and `commit`.
- **Pin**: the pack's `commit`; its installed snapshot is the pinned store
  directory.
- **Enabled**: a skill a run names in `skills:`; linked for that run only.
- **Skills collection**: the indexed pinned snapshots, `<namespace>-skills`.

## Proposed surface (as built)

### Config

```toml
[[skills.sources]]
name   = "pstack"                                  # used as pack/skill in orders
kind   = "mode"                                    # mode | tool | domain
url    = "https://github.com/cursor/plugins.git"
ref    = "refs/heads/main"                         # where reviewed commits come from
subdir = "pstack"                                  # searched for */SKILL.md, up to 5 levels
commit = "3fa33bb0f927bea2ccacd48a3cfda95145fb6bae" # the pin; required, full SHA
```

A source with any of `name`, `kind` or `commit` is a shelf pack, and all
three are required. Without them a source is an RFC 0012 team source, linked
into the home skill roots as before. A shelf pack's skills are found anywhere
under `subdir` (hidden directories and `node_modules` skipped; a directory
with SKILL.md is one skill and is not searched further), so nested layouts
such as `skills/engineering/tdd` work. Two skills with one name in a pack are
an error. Sync installs the exact commit, checks it is reachable from `ref`,
and fetches only when the commit is not in the mirror yet; the RFC 0012
fast-forward rule does not apply to shelf packs because a bump is explicit.
`git archive` is limited to the pack's subdirectory plus licence files from
the repository root down to it, so a symlink elsewhere in the upstream (three
of the seven starting repositories have one) does not break the install.

### Commands

| Command | What |
| --- | --- |
| `hev skills add OWNER/REPO[//SUBDIR][@REF] [--kind K] [--name N]` | Resolve the ref with `git ls-remote` (default branch when omitted; a full SHA pins that commit on the default branch), append the entry, sync, index. A failed sync restores the config. |
| `hev skills shelf [--json]` | Pack, kind, pinned commit, licence, author, URL at the pin, skills; per-skill licences when they differ. |
| `hev skills shelf diff PACK [--to SHA] [--patch]` | Fetch; commits on the ref touching the pack since the pin, a diffstat, optionally the full patch. Moves nothing. |
| `hev skills shelf bump PACK --to SHA` | Refuses without `--to`, refuses a ref name, refuses a commit not on the pack's ref; rewrites only that pack's `commit` line, syncs, re-indexes. |
| `hev skills shelf index` | Re-index all installed pins. |
| `hev skills search QUERY [--pack P] [--top N] [--json]` | Search the skills collection, scoped to installed pins. |

Accepted argument forms are those `npx skills add` takes for GitHub:
`owner/repo`, `owner/repo/sub/dir`, `owner/repo//sub/dir`, `@ref`,
`github:owner/repo`, `github.com/owner/repo`, `https://github.com/owner/repo[.git]`,
`https://github.com/owner/repo/tree/REF/sub/dir` and
`git@github.com:owner/repo.git`. The default name is the last subdir segment
that is not generic (`skills`, `plugins`, `packages`), else the repository,
else the owner.

### Orders and recipes

```yaml
---
name: try-pstack
host: mini
repo: kit
harness: claude
skills: [pstack/poteto-mode, hunk/hunk-review]
---
```

A context recipe may carry `"skills": ["pack/skill", ...]`; the run enables
the union. The target records the resolved skills (pack, skill, kind, commit,
licence, pinned directory) in `meta.json` as `shelf_skills`, and the worker's
prompt names them.

Linking: Claude Code reads `.claude/skills` in the worktree, Codex
`.agents/skills`. Dispatch creates the root if needed and writes a
`.gitignore` there (`*` when it created the root, otherwise `/.gitignore` and
one `/NAME` line per linked skill); a root that already has its own
`.gitignore`, or an existing skill of the same name, refuses the launch
rather than shadowing or editing a tracked file. Unix links are symlinks into
the pinned store; Windows gets copies. Every path dispatch creates is listed
in `skills.json` in the run record before it is made, removed newest first
when the harness exits, and undone before the next attempt if a supervisor
died. Validators get no shelf skills.

### Layer

Namespace `<namespace>-skills` (`hev-traces-skills` by default). Row id
`skill-` + SHA-256(source, commit, path, part)[:32]. Attributes: `text`
(embedded, full-text), `pack`, `skill`, `kind`, `source`, `url`, `commit`,
`path`, `part`, `license`, `author`, `homepage`; `pack` and `commit` are the
only filterable ones (`docs/schema-cut/policy.json`). Rows of a pin that was
bumped away stay until garbage collection but never answer, because every
search filters `commit In` the installed pins. Without a Layer key the same
chunks go to `~/.local/state/hev-skills/index.json` and search is lexical.

RFC 0007's collections (`hev traces search --in skills`) have not landed on
v0.7, so `hev skills search` is the smallest equivalent: the namespace,
`show` attributes and filter above are what the `[collections.skills]` entry
will name when `--in` lands.

### Capture

`skill_loads[].commit` is filled when the load's path is inside
`~/.local/opt/hev-skills-SOURCE-COMMIT/`, or is an absolute path whose
symlinks resolve there at capture time (a dispatch run's link, while the run
is live). Native Skill tool calls that record no path, relative paths and
links already removed stay empty, as skill capture requires.

## Reviewing before a bump

No scheduler is added. An orchestrator round or a `hev watch` job runs:

1. `hev skills shelf diff PACK --patch > diff.txt` for each pack.
2. When the diff is not empty, dispatch an independent review-only run whose
   order includes the diff and asks for a verdict on injected instructions
   (text addressed to the agent that changes its goals or permissions),
   exfiltration (network calls, credential reads, uploads), scope creep
   (new commands, hooks, MCP servers, `bin/` files) and licence changes.
3. Only on PASS does a person, or the orchestrator if the operator has
   authorized it, run `hev skills shelf bump PACK --to SHA` with the commit
   the review read. The bump re-indexes. Any other verdict leaves the pin.

See [docs/skills-shelf.md](../skills-shelf.md) for a concrete order.

## Open questions

- Garbage collection of old pins and their Layer rows (RFC 0012 retains pins).
- Whether the sender should check `skills:` against its own shelf before
  sending; today only the target can, because only it has the pins.
- Audience and machine targeting for shelf packs (RFC 0012 metadata is
  ignored for them, since nothing is linked into the home roots).
- Skill-level licence detection reads only licence files; a SKILL.md
  `license:` field is shown nowhere yet.
