# Runbooks

Operator recipes for running kit's `hev` binary as standing automation.

- [Windows host](windows-host.md): install `hev.exe`, supervise an
  orchestrator under a scheduled task with launch-time credentials, route
  owner notices from a host without peers, and avoid Task Scheduler and
  PowerShell 5 traps.
- [Follow-through orchestrator](follow-through-orchestrator.md): an
  agent-driven loop that reviews with a second harness, sends rework, checks
  CI, keeps named waits and lists merge-ready PRs for a person.
- [Carry-over](carry-over.md): adopt the runs a retiring loop holds with a
  seed checkpoint and an expiring, names-scoped spec, and roll back.
- [Eval corpus replay](eval-corpus-replay.md): replay a historical eval corpus
  into a new namespace.

Example specs live in [examples/](examples/). `go test ./cmd/hev` dry-runs
each of them with `hev orchestrate apply --dry-run`.
