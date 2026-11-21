# Dispatch workspaces

Starter Kit and community Layer use normal Git worktrees. A host with a
valid Pro Layer license can share build caches across attempts and reclaim
finished worktrees. The receiver decides eligibility from its local license
projection.

In the receiver's dispatch config:

```toml
cap = 8
[workspace]
mode = "auto"
root = "~/workspace"
driver = "cow"
auto_compact_done = false
```

`auto` enables Pro features when licensed. `oss` selects plain Git worktrees.
`pro` requires a valid Pro license. The legacy top-level string
`workspace = "~/workspace"` still sets the repository search directory.
The `cow` driver tries an APFS clone or Linux reflink and falls back to Git
checkout. Set `driver = "git"` to always use Git checkout.

On supported hosts each Pro attempt leases one slot per repository up to the
receiver's cap. Rust attempts with a root `Cargo.toml` inherit a slot's
`cargo-target` directory unless their environment already sets
`CARGO_TARGET_DIR`. `hev dispatch gc` evicts unlocked caches idle over seven days.

Run `hev dispatch compact NAME` or `hev dispatch compact --all` on a Pro host.
Closing an eligible run also attempts compaction. Enable
`auto_compact_done` to attempt it after successful work. Compaction requires
an idle run, a clean worktree (including untracked source), and a HEAD
reachable from origin or equal to the original base. Worktree and Git
administration paths must resolve within the host home directory.
Compaction keeps run metadata, reports, and review notes. Rework, retry,
and amend restore the recorded commit and branch before the next attempt.
