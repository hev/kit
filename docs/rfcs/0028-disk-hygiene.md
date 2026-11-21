# RFC 0028 — Native disk-pressure and cache hygiene in hevd

Status: Implemented

## Summary

Run bounded, opt-in host disk hygiene inside `hevd` and on demand via
`hev hygiene run` / `hev hygiene status` across Linux, macOS, and Windows.
Hygiene absorbs the cross-OS cleanup previously split across ad-hoc host scripts
(`disk-prune.sh`, `build-caches.py`): compacting old ended dispatch worktrees,
pruning stale current-UID temporary directories, and thinning macOS APFS local
Time Machine snapshots when free disk space drops below a configured threshold.

## Motivation

Dispatch hosts accumulate multi-gigabyte worktrees, stale temporary directories
from builds and tests, and macOS APFS local snapshots that prevent freed blocks
from returning to the volume. Relying on out-of-band shell scripts duplicates
scheduling, risks deleting active or unpushed work, and provides no unified
status or dry-run inspection.

## Decision

Add an opt-in `[hygiene]` block to `~/.hev/config.toml` (disabled by default)
and a supervised periodic loop inside `hevd`. Reuse `internal/dispatch`'s safe
compaction guards so hygiene never removes active, queued, recent, dirty,
unpushed, or out-of-home worktrees. Restrict temporary directory pruning to
current-UID-owned `tmp.*`, `hev-*`, and `go-build*` directories under
`os.TempDir()` older than `prune_tmp_after`, skipping symlinks and active lock
files. On macOS, invoke bounded `tmutil thinlocalsnapshots` with a strict
timeout only when free space on the dispatch volume falls below `min_free_gb`.

Persist minimal, path-free and secret-free state in `~/.hev/hygiene/state.json`
coordinated with an OS file lock. Dry-run mode evaluates all safety guards and
reports planned actions while mutating zero files.

## Goals

- Keep active (`running`, `queued`), recent, dirty, unpushed, and external-mount
  dispatch worktrees untouched.
- Compact clean ended (`done`, `failed`, `closed`) dispatch runs older than
  `compact_after`.
- Prune stale current-UID-owned `tmp.*`, `hev-*`, and `go-build*` directories
  under `os.TempDir()` without touching other users' files or active locks.
- Thin macOS APFS local snapshots with a bounded timeout only under disk
  pressure (`free < min_free_gb`).
- Compile, vet, and pass tests cleanly on Linux, macOS, and Windows.

## Non-goals

- Deleting dispatch run metadata (`meta.json`, `result.json`, `report.md`,
  `order.md`, logs, or review notes).
- Pruning arbitrary user directories or other users' temporary files.
- Automatic enablement without explicit `[hygiene] enabled = true`.
- Remote or cluster-wide coordinated disk pruning.

## Vocabulary

- **Disk pressure**: free space on the dispatch volume is strictly below
  `min_free_gb`.
- **Compaction**: guarded removal of a dispatch run's `wt/` checkout while
  pinning `head_sha` in `meta.json` so rework, retry, or amend can restore it.
- **Dry run**: full evaluation of all safety guards and planned actions with
  zero filesystem mutations and zero subprocess side effects.

## Proposed surface

See [Disk hygiene](../hygiene.md) for `[hygiene]` configuration, safety rules,
state format, and CLI commands (`hev hygiene run`, `hev hygiene status`).

## Open questions

- Whether future revisions should expose a per-host disk-pressure signal in
  cluster membership facts to steer dispatch placement away from low-disk hosts.
