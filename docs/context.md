# Composable context

`hev context` composes the instructions and skills an agent is given into an
explicit, reproducible bundle, and dispatch builds that bundle for each run
whose order names a recipe. Every source in a bundle is inspectable with its
provenance, and the bundle's digest ties the context a run was given to how
the run turned out.

This is Weave (`github.com/hev/weave`), brought into kit unchanged in design:
explicit ordered JSON recipes, no hidden defaults, deterministic bytes. Weave's
example recipes build bundles with the same digests under `hev context build`
as under `weave build` (`internal/weave/parity_test.go`).

## Commands

```sh
hev context assess  --root PROJECT --out NEW     # discover sources, write findings and recipes
hev context extract --root PROJECT --out NEW     # assess, and copy sources into NEW/sources
hev context build   --root DIR --recipe R.json --out NEW
hev context diff    --before A --after B --out NEW
hev context review  --bundle DIR [--addr 127.0.0.1:0]
hev context review  --run NAME [--attempt N]     # the bundle a dispatch run on this host was given
```

- `assess` discovers `AGENTS.md` and `CLAUDE.md` at any depth and the root
  `.claude/agents/`, `.claude/skills/` and `.agents/skills/` trees. It writes
  `assessment.json`, the baseline `recipe.json` and an editable
  `improved.recipe.json` that omits exact duplicate instruction files.
  Findings are deterministic lexical checks (duplicates, repeated lines,
  oversized files, missing `Scope:`, missing local links, opposite
  `Always`/`Never` directives) with source path and line.
- `build` writes `bundle.json` (manifest with every source's bytes, hash and
  target, plus the assessment and digest), `context.md` (the assignment and
  every Markdown source in recipe order), `materialized/` (sources at their
  targets, bytes unchanged) and a self-contained `index.html` reviewer.
- `diff` verifies both digests and writes `diff.json` and a before/after
  reviewer: assignment, source ordering, and added, removed or changed files.
- `review` serves only the generated `index.html` on a loopback address. It
  never launches a browser; the static page works offline too.

Recipes are ordinary JSON with no templating:

```json
{
  "version": 1,
  "task": "Review the storage change",
  "sources": [
    {"path": "AGENTS.md", "target": "AGENTS.md"},
    {"path": ".claude/agents/reviewer.md", "target": ".claude/agents/reviewer.md"}
  ]
}
```

Paths are relative to `--root`, targets relative to `materialized/`. Ordering
is explicit and part of the digest. Unknown fields and trailing values are
refused.

Safety checks are Weave's: absolute paths, traversal, backslashes, symlinked
sources (including any symlinked ancestor), duplicate and overlapping targets,
non-regular files and sources over 16 MiB are refused. Every output must be a
new directory whose parent exists; existing outputs and sources are never
overwritten. Source HTML and Markdown are shown as text in the reviewer, never
rendered. Bundles contain full source contents: keep them where those
contents belong.

## Context recipes in dispatch orders

Name a recipe in an order's frontmatter:

```markdown
---
name: improve-help
host: buildbox
repo: kit
harness: claude
context: .hev/context/review.json
---
Improve the help text.

## Acceptance
- Help explains the new command and tests pass.
```

`context` is a path inside the order's repository, under the same rules as a
recipe source path. The target reads it with Git at the run's base commit:
the fork point of the run's branch from `origin/<base>`, which for a new run
is `origin/<base>` itself. That commit is pinned with the first bundle and
reused by later attempts on the same base, so a worker cannot change the
recipe its next attempt is given. An amendment that changes `base` resolves
the commit again. The recipe must be a regular file in that commit.

The recipe's `task` is replaced by the order body (everything after the
frontmatter, including Acceptance and any `## Rework` notes): the order says
what the attempt is for. Sources are read from the run's worktree, so a
rework sees the worker's own changes to instruction files.

An order without `context` behaves exactly as before.

### When the bundle is built

On the receiving host, after the worktree exists and before the supervisor
starts the harness, for every launch: `start` (including a dequeued order),
`rework`, `retry`, an ended-run `say`, an `amend` that starts a new attempt,
and the import side of a host move. An amendment that changes `context` on a
live run stops and restarts it, like a model change, because a worker receives
its context only at launch. Validators get no bundle in this slice.

The bundle is published into the run record, never the worktree, as
`~/dispatch/<name>/contexts/<n>/`, with the record's permissions (directories
`0700`, files `0600`). `n` counts up; earlier attempts' bundles are never moved
or replaced.

A missing recipe, invalid recipe, or a source the recipe names that does not
exist fails the run before launch with an error naming the problem, and
nothing is launched:

- a new run fails admission (`show`/`ls --all` keep the error; `clean` removes it);
- a rework, retry, `say` or amend records the attempt with a failed
  `result.json` carrying the error;
- a host move's import is refused and the source run is restored.

The supervisor checks again before starting the harness: it loads the
attempt's bundle, verifies its digest against the record and that
`context.md` matches the manifest, and refuses to launch otherwise. A run is
never launched with a partial bundle or without the context its order names.

### How the worker receives it

The harness gets the bundle's `context.md`, byte for byte, on standard input.
The prompt argument is unchanged.

- `claude -p PROMPT` reads standard input as input alongside the prompt.
- `codex exec PROMPT` appends standard input to the prompt as a `<stdin>` block.

This works the same way on Unix (tmux supervisor) and Windows (job-object
supervisor). A `harness_wrapper` must pass standard input through to the
harness: `shift; exec "$@"` on Unix does, and so does the PowerShell 7
`ProcessStartInfo` wrapper in [dispatch.md](dispatch.md), which does not
redirect it. Both harnesses were checked reading a regular file on standard
input (`claude` 2.1.321, `codex-cli` 0.162.0).
Runs without a recipe get no standard input, as before. Each round of a
repeating order is given the same attempt's bundle.

`context.md` starts with `# Assignment` and the order body, followed by each
selected Markdown source under a `# Source: PATH → TARGET` heading. Non-Markdown
assets (skill files, checklists) are in the bundle's `materialized/` tree and
manifest but not in `context.md`.

### What the harness still loads on its own

The bundle is the context dispatch gave the worker explicitly. It is not
everything the harness saw. Claude Code and Codex also load `AGENTS.md`,
`CLAUDE.md` and skills from the worktree, user-level configuration such as
`~/.claude` and `~/.codex`, and their own system prompts. Dispatch does not
suppress or measure that. Each record lists, under `harness_also_loads`, the
instruction and skill files present in the worktree when the bundle was built,
and carries a note saying the bundle is not the whole picture.

### What is recorded

`meta.json` gains, for the current attempt:

- `context` (from the order): the recipe path;
- `context_digest`: the bundle digest;
- `context_bundle`: `digest`, `recipe`, `recipe_sha256` (of the recipe bytes
  read), `recipe_commit`, `base`, `bundle` (`contexts/<n>`), `manifest`
  (`contexts/<n>/bundle.json`, the full source manifest), `context`
  (`contexts/<n>/context.md`), `sources` (path, target, sha256 and bytes of
  each source), `delivery` (`stdin`), `harness_also_loads`, `note` and `built`.

`result.json` gains `context_digest`, so an archived attempt's result still
names the bundle it was given. `history.json` keeps each attempt's full
`meta`, including its `context_bundle`.

```sh
hev dispatch show NAME              # Context: digest, recipe, bundle; each attempt's digest
hev dispatch show NAME --json       # files["meta.json"] and files["result.json"] carry it
hev dispatch ls --json              # rows carry context_digest and context_bundle
hev context review --run NAME       # open the current attempt's bundle (on the run's host)
hev context review --run NAME --attempt 1
```

Records written before context recipes have none of these fields and are read
as before. The fields are optional and ignored by older `hev` versions.

### Joining sessions to their context

During indexing, a worker session under `~/dispatch/<name>/` receives
`context_digest` on its session row: the digest of the attempt whose result
recorded that session ID, or the run's current attempt otherwise. It is a new
filterable attribute on the sessions namespace.

```sh
hev traces ls --context DIGEST
hev traces search 'why did it fail' --context DIGEST
```

`traces ls --context` filters session rows directly and, like `--order`,
searches all ages unless a window is chosen. `traces search --context` finds
the sessions given that digest and limits chunks to them (chunks do not carry
the digest themselves).

### Older hosts

New `meta.json` and RPC fields are optional and ignored by older senders and
targets. An older target parses orders strictly, so it refuses an order with
`context:` (`unknown key: context`) before admitting it; it never launches
without the context. The sender reports this as the target's version not
supporting context recipes. Upgrade `hev` on the target first. `hev dispatch
hosts` shows each target's version.

### Recipe growth

Recipes are version 1. Kit refuses unknown recipe fields, so a future source
kind (for example a search over traces or the board) can be added as a new
optional field without a version break: existing recipes and their digests do
not change as long as new bundle fields are omitted when unused, and an older
`hev` given a recipe that uses the new kind refuses it rather than ignoring it.
