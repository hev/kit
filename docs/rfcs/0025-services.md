# RFC 0025 — Native services in hevd

Status: Implemented

## Summary

Supervise long-lived argv processes inside `hevd` across macOS, Linux and
Windows using `[[service]]` in `~/.hev/config.toml` and the `hev service` CLI,
sharing the daemon's in-memory secrets cache, attention pipeline and optional
SWIM active-standby placement.

## Motivation

`hevd` already runs one-shot orders (`hev dispatch`), recurring schedules
(`hev cron`) and stateful agent loops (`hev orchestrate`), while long-lived
cluster services (`hev board serve`, `intranet`, `company`, private previews
and local sidecars) still required separate `launchd` plists, `systemd --user`
units and Windows Scheduled Tasks on each host. Per-OS service definitions
duplicate secret wiring, log retention, crash reporting and failover logic.

## Decision

Declare long-lived processes once under `[[service]]` in `~/.hev/config.toml`
and supervise them in `internal/services` inside `hevd`. Persist private
per-service runtime state in `~/.hev/services/<name>/state.json` and bounded,
redacted output in `~/.hev/services/<name>/service.log` (capped at 1 MiB).
Control local services synchronously through `hev service` (`ls`, `show`,
`logs`, `start`, `stop`, `restart`) coordinated with `hevd` via private file
locks under `~/.hev/services/`.

## Goals

- Eliminate per-OS service units for long-lived kit and workspace daemons.
- Execute direct argv vectors (`exec.CommandContext`, never a shell) in an
  isolated Unix process group or Windows Job Object so stops and `hevd`
  shutdown leave no orphaned descendants.
- Resolve `env_refs` at start and restart through `internal/secrets` and scrub
  resolved secret values, gitleaks credentials and local paths from logs,
  `state.json` and CLI output.
- Bound restart loops with exponential backoff (`2s` to `5m`, resetting after
  60s of healthy uptime) and emit one deduplicated `done` (`failed`) attention
  notice when a service crashes repeatedly (>= 3 rapid exits within 2m).
- Support optional SWIM rendezvous (HRW) active-standby placement so only the
  winning alive member runs a placed service and non-winners stop cleanly when
  ownership moves.

## Non-goals

- Arbitrary shell pipelines, container orchestration, port proxying or multi-replica load balancing.
- Distributed consensus or split-brain fencing beyond SWIM membership convergence (services using `placement` should tolerate brief overlap during network partitions).
- Remote command distribution over the network: every host executes only the `[[service]]` entries in its own local `config.toml`.

## Vocabulary

- **Service**: a configured `[[service]]` entry supervised by `hevd`.
- **Crash loop**: three or more rapid exits (uptime <= 60s) within a rolling two-minute window.
- **Placement winner**: the alive member in `placement` with the highest SHA-256 rendezvous score for `(service, member)` in this node's SWIM view.

## Proposed surface

See [services](../services.md) for configuration fields, supervision rules,
redacted ring log behavior, crash-loop notices, SWIM placement and CLI
commands.

## Open questions

- Whether a later phase should add optional HTTP/TCP readiness probes before resetting restart backoff or advertising service endpoints in SWIM metadata.
