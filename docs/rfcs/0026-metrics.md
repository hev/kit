# RFC 0026 — Native cluster metrics export in hevd

Status: Implemented

## Summary

Export a strictly allowlisted set of cluster, secrets-quota, orchestrator, and
cron gauges directly from `hevd` via DogStatsD UDP and an optional Prometheus
`/metrics` HTTP endpoint, configured under `[metrics]` in `~/.hev/config.toml`.

## Motivation

External polling scripts such as `hev-cluster-metrics.py` duplicate daemon
scheduling, shell out to `hev cluster ls` / `hev secrets status` / `hev
orchestrate ls` / `hev cron ls` on every tick, risk leaking arbitrary CLI text
into telemetry pipelines, and require host-specific cron or systemd wrappers.
`hevd` already holds the live SWIM view, the in-memory secrets cache and budget
ledger, and the cron runner in-process, so native export eliminates subprocess
overhead, ensures consistent privacy boundaries, and works identically across
Linux, macOS, and Windows.

## Decision

Add an opt-in `[metrics]` table in `~/.hev/config.toml` (disabled by default)
backed by `internal/metrics` and wired into `internal/daemon`:

1. **Private-only network guard.** Both `dogstatsd` (UDP destination) and
   `prometheus_addr` (HTTP listen address) must be explicit IP literals on
   loopback (`127.0.0.0/8` or `::1`) or Tailscale CGNAT (`100.64.0.0/10`).
   Wildcard binds (`0.0.0.0`, `::`), public IPs, RFC 1918 LAN addresses, and
   hostnames are refused at config validation and transport construction time.
2. **Strict metric and tag allowlist.** Only twelve fixed gauges and three
   fixed tag keys (`member`, `source`, `state`) may ever be serialized. Every
   sample passes through a central validator before DogStatsD or Prometheus
   encoding; unknown metric names, unknown tag keys, non-allowlisted states,
   non-finite numbers, and invalid member identifiers are dropped.
3. **Optional SWIM witness placement.** When `placement` is non-empty, cluster-
   wide `hev.cluster.*` gauges are emitted only by the single alive member in
   `placement` with the highest rendezvous hash score, preventing N-way
   duplicate cluster series while still letting every host emit its own local
   `hev.secrets.*`, `hev.orchestrator.by_state`, and `hev.cron.failing` gauges.
4. **Bounded I/O and clean shutdown.** UDP writes enforce a `<= 2s` deadline
   and cap datagrams at 1024 bytes (safe for Tailscale WireGuard MTU). The
   Prometheus HTTP server enforces `ReadHeaderTimeout`, `ReadTimeout`,
   `WriteTimeout`, `IdleTimeout`, and `MaxHeaderBytes`, and shuts down
   gracefully when `hevd` exits.

## Goals

- Retire external metrics scraping scripts like `hev-cluster-metrics.py`.
- Zero subprocesses and zero provider API calls during metrics collection
  (uses `secrets.Cache.LocalStatus()`, in-memory SWIM membership, and local
  state files only).
- Structurally prevent secret values, references, local file paths, repository
  names, prompts, or arbitrary user strings from appearing in emitted metrics
  or configuration errors.
- Work identically on Linux, macOS, and Windows with isolated, deterministic
  tests.

## Non-goals

- General-purpose histograms, timers, custom user-defined metrics, or arbitrary
  label cardinality.
- Authenticated public-internet Prometheus endpoints or TLS termination inside
  `hevd`.
- Pushing metrics over HTTPS/OTLP directly to remote SaaS APIs (use a local or
  Tailscale DogStatsD agent or Prometheus scraper).
- Distributed locking or exactly-once witness handoff during SWIM partitions.

## Vocabulary

- **Cluster-wide gauges:** `hev.cluster.*` metrics tagged `member:<name>,source:swim`,
  one series per known SWIM member.
- **Host-local gauges:** `hev.secrets.*`, `hev.orchestrator.by_state`, and
  `hev.cron.failing` metrics tagged `member:<self>`, reflecting this host's own
  local state.
- **Witness:** The alive member in `[metrics] placement` with the highest
  SHA-256 rendezvous score for `"metrics\x00" + member.Name` in this node's
  SWIM view.

## Proposed surface

See [metrics](../metrics.md) for the `[metrics]` TOML schema, validation rules,
complete gauge and tag allowlist, DogStatsD and Prometheus text formats, and
migration notes.

## Open questions

- Whether a future increment should add an optional `hev metrics check` or
  `hev daemon status` summary line showing active sinks and last emission time.
- Whether Tailscale IPv6 ULA (`fd7a:115c:a1e0::/48`) should be added to the
  private bind guard once dual-stack Tailscale clusters are in use.
