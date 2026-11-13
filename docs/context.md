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
hev context plan    ORDER.md --root REPO --out NEW   # inventory and starting recipe for an order
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

### Narrowing a file by heading

A Markdown source can keep or drop sections instead of being taken whole:

```json
{"path": "AGENTS.md", "target": "AGENTS.md", "drop": ["## Release process", "## Billing"]}
{"path": "CLAUDE.md", "target": "CLAUDE.md", "keep": ["## Testing", "### Fixtures"]}
```

- A selector is an ATX heading written as it appears in the file, hashes
  included: `"## Testing"` matches every level-2 heading whose text is
  `Testing`. Whitespace is trimmed and collapsed, and a closing run of `#` is
  ignored. `"(preamble)"` names the text before the first heading. Headings
  inside fenced code blocks, and Setext (underlined) headings, are text.
- A heading's section runs to the next heading of the same or a higher level,
  so it includes its subsections.
- `keep` keeps the preamble, each matched section, and the heading lines of its
  enclosing sections (so a kept `### Fixtures` still sits under its `##`);
  everything else goes. `drop` removes each matched section and is applied
  after `keep`.
- A selector that matches no heading is an error, and so is a result with
  nothing left. Selectors are refused on non-Markdown sources.

Why headings: they are the structure instruction files already have, a person
can read a selector and know what it means, and resolving them needs no model
and no line numbers that drift with every edit. Narrowing only strips: the
result is the source's own bytes, in order, with nothing rewritten or
summarised. The same recipe and file always give the same bytes.

A narrowed file in `bundle.json` carries a `selection`: the full file's
`source_sha256` and `source_bytes`, the `keep` and `drop` selectors, and the
`kept` and `dropped` sections (heading, level, line, lines, bytes). Its
`sha256`, `bytes` and `data` are the narrowed bytes, and `context.md` labels it
`(narrowed: N of M bytes)`. The reviewer page lists kept and dropped sections
for each narrowed file. Whole-file sources have no `selection`, so their bundles
and digests are unchanged (`internal/weave/parity_test.go`). `hev context
build` prints each narrowed file's size before and after.

### Planning a recipe for an order

`hev context plan ORDER.md --root REPO --out NEW` runs the assessment on the
order's repository and writes:

- `recipe.json`: every discovered source, whole, with the order body as its
  task (it builds as is);
- `plan.json`: each source's kind, bytes, hash, sections (with sizes including
  subsections) and findings;
- `index.html`: a review page with the same, and each file's full text.

It calls no model and cuts nothing. Choosing what to keep is the reviewer's
job: the `hev-context` skill walks an agent and the person it works for
through it before a dispatch. A repository with no instruction files prints
`nothing to narrow` and writes nothing.

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
what the attempt is for. Sources are read from the run's worktree (with any
files a previous attempt replaced put back first), so a rework sees the
worker's own committed changes to instruction files.

An order without `context` or `context_recipe` behaves exactly as before.

### A recipe for one dispatch

The sender can also pass a local recipe for this dispatch only:

```sh
hev dispatch order.md --context-file narrowed.json
hev dispatch run REPO 'Fix the login redirect' --context-file narrowed.json
hev dispatch amend NAME order.md --context-file narrowed.json
```

The sender checks the file is a recipe (strictly: unknown fields such as a
`digest` or bundle are refused) and adds it to the order's frontmatter as
`context_recipe: BASE64`. It travels with the order: queued, admitted, amended
and moved between hosts like any other order field. An order may have
`context` or `context_recipe`, not both.

On the target, the first build pins the recipe in the run record as
`context-recipe.json` (`0600`) and records its SHA-256 in `meta.json`
(`context_recipe_sha256`) and the bundle record (`recipe_sha256`, with
`origin: "order"`). Every later attempt (rework, retry, an ended-run `say`)
reads the pinned copy and checks it against that hash, whatever `../order.md`
says by then, so a worker cannot change the recipe its next attempt gets. An
amendment pins the amended order's recipe again, and a host move pins it on the
new host. A sender-supplied `context_recipe_sha256`, bundle or digest is
discarded at admission. Sources are still read from the repository, in the
run's worktree, exactly as for a committed recipe.

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

### Instruction files in the worktree

Harnesses read `AGENTS.md`, `CLAUDE.md`, agents and skills from the worktree
themselves, so giving the worker a narrowed copy on standard input is not
enough: it would still load the full file. For a recipe-driven run the
supervisor puts each repository instruction file in place in the form the
recipe gives it, for exactly as long as the harness runs. The same mechanism
covers `claude` and `codex`, on Unix and Windows, with no harness flags.

Each file `assess` would discover (`AGENTS.md` and `CLAUDE.md` at any depth and
the `.claude/agents/`, `.claude/skills/` and `.agents/skills/` trees) gets one
form:

| Form | When | What the harness can read |
| --- | --- | --- |
| `narrowed` | the recipe names it with `keep` or `drop` | the bundle's narrowed bytes |
| `whole` | the recipe names it without selectors, or it belongs to a skill whose other files the recipe names | the file, untouched |
| `excluded` | the recipe does not name it | nothing: the file is removed |
| `full` | it would be narrowed or excluded but is untracked or has uncommitted changes | the file, untouched; the record gives the reason |

So the rule for files a recipe does not mention is: excluded. A recipe from
`hev context plan` names every file, so it starts from the status quo and every
removal is a decision. Narrowing changes only what that worker sees in its own
worktree. The repository, the base commit, the source checkout the worktree was
made from and anything the worker commits keep the full files.

How the replaced files stay out of git: before the harness starts, the
supervisor marks each replaced file `skip-worktree` in the worktree's own index
(worktrees have one index each; the source checkout's is untouched), then
writes the narrowed bytes or removes the file. Git then treats those paths as
matching the index: `git status`, `git diff`, `git add -A`, `git commit -a` and
`git stash` do not see them, and an explicit `git add PATH` is refused. When
the harness exits, the supervisor clears the marks and checks the files out
from the index again, so the worktree is back to its committed state before the
result is written. The state lives in the run record (`instructions.json`),
never in the worktree. If a supervisor dies with files replaced, the next
launch (rework, retry, `say`, amend restart) puts them back before it reads the
worktree to build its bundle. A repeating order replaces and restores them
around each round.

The worker is told. When any file is narrowed or excluded, the assignment ends
with a short `## Instruction files in this worktree (from dispatch)` section
listing them and saying how to restore one:
`git update-index --no-skip-worktree -- PATH && git checkout -- PATH`.

- **A task that edits an instruction file.** The review keeps that file whole,
  so it is an ordinary tracked file. If the worker needs to edit a narrowed or
  excluded one anyway, it restores the full file with the command above, edits
  and commits it as usual; the supervisor leaves a file the worker restored
  alone.
- **An accidental edit of a replaced file** is invisible to git and cannot be
  committed. After the run the supervisor saves the changed copy under
  `instruction-edits/<time>/PATH` in the run record, restores the file and
  reports it in `result.json` as `instruction_warnings`.
- **A rebase or checkout that changes a replaced file** may stop with "local
  changes would be overwritten"; the note tells the worker to restore the file
  first. If git writes the full file back itself, the worker sees the full file
  for the rest of the run; nothing is committed that should not be.

### What is recorded

`meta.json` gains, for the current attempt:

- `context` (from the order): the recipe path;
- `context_digest`: the bundle digest;
- `context_bundle`: `digest`, `recipe`, `recipe_sha256` (of the recipe bytes
  read), `recipe_commit`, `base`, `bundle` (`contexts/<n>`), `manifest`
  (`contexts/<n>/bundle.json`, the full source manifest), `context`
  (`contexts/<n>/context.md`), `sources` (path, target, sha256 and bytes of
  each source), `delivery` (`stdin`), `harness_also_loads`, `note` and `built`;
  a narrowed source adds `source_sha256`, `source_bytes`, `kept` and
  `dropped` (headings);
- `context_bundle.instructions`: each discovered instruction, agent and skill
  file with `path`, `kind`, `covered`, `form`, the `sha256` and `bytes` the
  harness could read (none for `excluded`), the full file's `source_sha256`
  and `source_bytes`, and a `reason` for `whole` or `full` when it is not
  obvious; `uncovered` (`excluded`, the rule for files the recipe does not
  name) and `in_place` (`skip-worktree`); `origin` is `order` for a recipe
  carried in the order;
- `context_recipe_sha256`: the pinned per-dispatch recipe's hash.

`harness_also_loads` now lists the discovered files the harness could still
read (every form but `excluded`). The record makes no claim about what else a
harness loads: user-level configuration such as `~/.claude` and `~/.codex`,
its system prompt and built-in context, or other files it reads from the
worktree (for example `CLAUDE.local.md` or `.claude/rules/`, which discovery
does not cover). The `note` says so.

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
`context:` or `context_recipe:` (`unknown key: ...`) before admitting it; it
never launches without the context. The sender reports this as the target's
version not supporting context recipes. A target that knows `context:` but not
narrowing refuses a recipe with `keep` or `drop` (`unknown field`) before
launch rather than giving the worker whole files. Upgrade `hev` on the target
first. `hev dispatch hosts` shows each target's version.

### Recipe growth

Recipes are version 1. Kit refuses unknown recipe fields, so a future source
kind (for example a search over traces or the board) can be added as a new
optional field without a version break: existing recipes and their digests do
not change as long as new bundle fields are omitted when unused, and an older
`hev` given a recipe that uses the new kind refuses it rather than ignoring it.
