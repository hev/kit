# Pro dispatch workspaces

Status: Implemented

## Summary and motivation

Parallel worktrees duplicate multi-gigabyte build outputs and retain them
after dispatch ends.

## Decision

Keep Git worktree semantics in both editions. Pro adds bounded attempt-scoped
cache leases, opportunistic copy-on-write checkout, and guarded worktree removal.
The receiving host is the authority for licensing, cap and local paths.
A failed safety check keeps the worktree. A compacted run records its exact
commit for reopening.

## Surface

See [Dispatch workspaces](../pro-workspaces.md) for configuration, commands
and lifecycle guarantees.
