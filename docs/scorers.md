# Scorers

`hev score` manages host-pinned scorers and runs synchronous commit evaluations
([RFC 0021](rfcs/0023-climb.md#5-what-rfc-0021-needs-to-add),
[RFC 0023](rfcs/0023-climb.md)).

A **metric** scorer evaluates a repository commit with a host-owned script or
executable and scales the numeric result to an integer mark compatible with
[trace evaluations](rfcs/0005-marks.md). Because candidate branches in a climb
or benchmark commit directly to the target repository, scorers never run from
the candidate checkout and never trust unpinned files: the host pins the scorer
spec, its command, and every declared data file by SHA-256 before running it.

## Metric scorer files

A scorer file is Markdown with strict YAML frontmatter (unknown keys are
rejected). Host scorers live under `~/.config/hev/scorers/<name>/scorer.md`:

```markdown
---
name: trace-recall-at-10
description: recall@10 of hev traces search on the pinned query set
kind: metric
subject: commit
command: score.py
direction: max
unit: ratio
scale: 1000
timeout: 30m
data:
  - snapshot/archive-2026-11-15.tar
  - queries/objective.jsonl
hidden: false
protect:
  - pkg/search/testdata/**
  - scripts/eval-*
---
Build the candidate checkout, evaluate recall@10 over `queries/objective.jsonl`,
and write `{"value": <ratio>}` to stdout.
```

### Frontmatter fields

| Field | Required | Meaning |
|---|---|---|
| `kind` | Yes | Must be `metric`. |
| `subject` | No | Target subject kind; defaults to `commit` (only `commit` is accepted). |
| `name` | No | Lowercase slug (`^[a-z][a-z0-9-]*$`). Defaults to the parent directory name for `<name>/scorer.md`, or the file stem. |
| `description` | No | Short summary of what the metric measures. |
| `command` | Yes | Host executable or script path (resolved relative to the scorer file's directory when relative). |
| `direction` | Yes | `max` (higher is better) or `min` (lower is better). |
| `unit` | No | Display unit for human summaries (for example `ratio`, `ms`, `hosts`). |
| `scale` | No | Positive integer multiplier turning `value` into an integer mark (`round(value * scale)`). Defaults to `1`; `0` and negative values are refused. |
| `timeout` | Yes | Bounded positive Go duration up to `24h` (for example `30s`, `30m`, `2h`). |
| `data` | No | Host paths read by `command` (resolved relative to the scorer file's directory when relative). Every entry is included in the SHA-256 pin set. |
| `hidden` | No | Boolean (`false` by default; `visibility: hidden` or `visibility: visible` is also accepted). Marks held-out scorers whose values and fixtures stay out of worker briefs. |
| `protect` | No | Repository-relative paths or globs (`*`, `?`, `[...]`, and full-component `**`) that a candidate diff may not modify or rename. |

## Host-side SHA-256 pin sets

Shipping a scorer file on disk or inside a repository grants it no authority to
run. An operator pins the scorer on the host first:

```sh
hev score pin ~/.config/hev/scorers/trace-recall-at-10/scorer.md
hev score verify ~/.config/hev/scorers/trace-recall-at-10/scorer.md
```

- `hev score pin <scorer-file> [--json]` validates the spec, computes SHA-256
  digests for the spec file, its resolved `command`, and every resolved `data`
  file, writes the composite pin set atomically to the host pin store
  (`~/.config/hev/scorers/pins.toml`, or `$HEV_SCORE_PINS`), and prints the
  recorded digests. Leaf symlinks and non-regular files are refused.
- `hev score verify <scorer-file> [--json]` checks the host pin store against
  current files on disk and exits non-zero when the scorer is unpinned or when
  any pinned file was modified, removed, or re-pointed (`stale`).
- `hev score run` verifies the pin set **before** touching the repository and
  **again after** `command` exits. Any mismatch refuses the run.

## Protected-path disqualification

When `--parent <rev>` is supplied, `hev score run` inspects changes across
`parent...commit` (including both the old path and the new path on renames and
copies) before creating a checkout or starting `command`:

- If any changed or renamed file matches a `protect` pattern (from the scorer
  spec or `--protect`), the commit is immediately **disqualified**.
- `command` is never invoked, no numeric `value` or `mark` is produced, the
  matched protected paths are reported in `protected_paths`, and `hev score run`
  exits with code `2`.
- Edits to other files — including ordinary neighbouring unit tests outside
  `protect` — proceed to scoring normally.

## Running a commit scorer

```sh
hev score run ~/.config/hev/scorers/trace-recall-at-10/scorer.md \
  --commit 8a41f0c \
  --parent v0.7 \
  --repo ~/workspace/kit \
  --json
```

### Isolation, bounds and output format

1. **Detached temporary checkout.** `hev score run` checks out `<rev>` into a
   fresh temporary directory (`wt` under a private `0700` scratch directory)
   with `origin` removed, and deletes the directory when the run finishes. The
   source repository is not modified.
2. **Host command and minimal environment.** It executes the pinned host copy
   of `command` with working directory set to the temporary checkout. The child
   process receives only a minimal environment (`PATH`, `LANG`/`LC_ALL`, fresh
   scratch `HOME` and `TMPDIR`, and `HEV_SCORE_SCORER`, `HEV_SCORE_COMMIT`,
   `HEV_SCORE_WORKTREE`, `HEV_SCORE_DATA_DIR`, `HEV_SCORE_DATA`,
   `HEV_SCORE_DATA_0`…), never host API keys or credentials.
3. **Timeout and output caps.** When `timeout` expires, the scorer process
   group is killed immediately and the run fails closed. Standard output is
   capped at 1 MiB and standard error at 64 KiB. Secret patterns, sensitive
   environment values, and local scratch paths are redacted from error messages
   and details.
4. **Numeric output.** `command` writes either a JSON object with a finite
   numeric `value` (plus optional `n` and `detail`):
   ```json
   {"value": 0.851, "n": 400, "detail": {"recall@10": 0.851, "p90_ms": 118}}
   ```
   or a single finite number (`0.851`). `hev score run` scales `value` by
   `scale` (`round(value * scale)`) to produce the integer `mark` (and
   `marks.<scorer_name>`).
