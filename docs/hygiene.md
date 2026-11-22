# Disk hygiene

`hevd` can run periodic, bounded disk-pressure and cache hygiene on a host, and
you can run or inspect it on demand with `hev hygiene`. Hygiene is opt-in:
background ticks only run when `[hygiene] enabled = true` is set in
`~/.hev/config.toml` (or the file selected by `HEV_CONFIG`).

## Configuration

Add a `[hygiene]` section to `~/.hev/config.toml` and restart `hevd`:

```toml
[hygiene]
enabled = true
every = "6h"
min_free_gb = 40
compact_after = "24h"
prune_tmp_after = "24h"
thin_apfs_snapshots = true
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enables the supervised hygiene loop in `hevd`. `hev hygiene run` still works on demand when `false`. |
| `every` | Go duration | `"6h"` | Interval between background hygiene ticks. Minimum `"15m"`. |
| `min_free_gb` | int | `40` | Free-space threshold (GiB) on the dispatch volume. Falling below this threshold marks the host under disk pressure and triggers macOS APFS snapshot thinning when enabled. |
| `compact_after` | Go duration | `"24h"` | Minimum age after a dispatch run ends before its clean worktree is eligible for compaction. |
| `prune_tmp_after` | Go duration | `"24h"` | Minimum age before stale matching directories under `os.TempDir()` are pruned. |
| `thin_apfs_snapshots` | bool | `true` on macOS, `false` elsewhere | When under disk pressure on macOS, runs `tmutil thinlocalsnapshots <volume> <bytes> 4` with a bounded timeout. |

## Safety rules

### Dispatch worktree compaction

Hygiene reuses `internal/dispatch`'s compaction guards:

- Only ended runs in `done`, `failed`, or `closed` states whose end (or close)
  timestamp is at least `compact_after` old are considered.
- Active (`running`), `queued`, `halted`, `blocked`, `needs-review`, `timeout`,
  `died`, or `launch-unconfirmed` runs are always kept.
- Worktrees with uncommitted changes, untracked source files (outside
  `target/` and `node_modules/`), hidden index flags, unpushed HEAD commits,
  or Git administration paths resolving outside `HOME` (such as external
  `/Volumes` mounts) are always kept.
- Compaction removes only `wt/` and records `head_sha` in `meta.json`; run
  metadata, orders, reports, logs, and review notes remain intact so rework,
  retry, or amend can restore the worktree.

### Temporary directory pruning (Linux / macOS)

- Inspects direct child directories of `os.TempDir()` only (never follows
  symlinks).
- Matches only `tmp.*`, `hev-*`, and `go-build*` directory names.
- Requires ownership by the current UID (`os.Getuid()`). Other users' files and
  non-matching names are never touched.
- Requires the directory and its top-level entries to be older than
  `prune_tmp_after`.
- Skips any directory containing an actively locked `.lock` or `lock` file.

### macOS APFS local snapshot thinning

- Runs only on `darwin`, only when `thin_apfs_snapshots = true`, and only when
  free space on the dispatch volume is strictly below `min_free_gb`.
- Invokes `tmutil thinlocalsnapshots <volume> <bytes> 4` with a strict 2-minute
  timeout and bounded reclaim target.

### Dry-run guarantee

`hev hygiene run --dry-run` evaluates every safety guard and reports exact
planned actions and estimated bytes without removing any worktree, deleting any
temporary directory, invoking `tmutil`, or writing state files.

## State

Each non-dry-run pass writes a summary to `~/.hev/hygiene/state.json` (mode
`0600` in a `0700` directory, serialized with `~/.hev/hygiene/run.lock`). State
records only timestamps, byte counts, action counters, pressure status, and
sanitized error messages—no secrets or local filesystem paths.

## Commands

- `hev hygiene run [--dry-run] [--json]` — executes one bounded hygiene pass
  (or plans it in `--dry-run`) and prints freed bytes, compacted runs, pruned
  temporary directories, and APFS snapshot thinning results.
- `hev hygiene status [--json]` — prints the last hygiene run time, free space
  before/after, current free space, pressure state, and configured thresholds.
