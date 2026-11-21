# hevd metrics export

`hevd` can export a strictly allowlisted set of cluster, secrets-quota,
orchestrator, and cron gauges directly over DogStatsD UDP and an optional
Prometheus `/metrics` HTTP endpoint. This replaces external polling scripts
such as `hev-cluster-metrics.py` without spawning subprocesses or spending
1Password API quota.

Metrics export is **opt-in** (`enabled = false` by default). When disabled or
absent, `hevd` starts no metrics goroutines and binds no sockets.

## Configuration

Add a `[metrics]` table to `~/.hev/config.toml` (or the file selected by
`HEV_CONFIG`), then restart `hevd`:

```toml
[metrics]
enabled = true
interval = "60s"
dogstatsd = "127.0.0.1:8125"
prometheus_addr = "127.0.0.1:9108"
placement = ["oven", "mini"]
```

### Fields and validation

| Field | Type | Default | Rules |
|-------|------|---------|-------|
| `enabled` | bool | `false` | Opt-in switch for metrics export. |
| `interval` | Go duration string | `"60s"` | Minimum `"10s"`, maximum `"1h"`. Controls the DogStatsD push cadence. |
| `dogstatsd` | `IP:port` string | `""` | Optional UDP destination. Must use an explicit loopback (`127.0.0.0/8`, `::1`) or Tailscale CGNAT (`100.64.0.0/10`) IP with a non-zero port (`1`–`65535`). |
| `prometheus_addr` | `IP:port` string | `""` | Optional HTTP listen address serving `GET /metrics`. Same loopback / Tailscale CGNAT IP guard (`0.0.0.0`, `::`, public IPs, RFC 1918 LAN IPs, and hostnames are refused). Port `0` is accepted for isolated tests. |
| `placement` | `[]string` | `[]` | Optional list of up to 64 distinct SWIM member names (`[A-Za-z0-9._-]`, max 64 bytes). Controls witness election for cluster-wide gauges. |

Invalid `[metrics]` settings — such as `interval < 10s`, `interval > 1h`,
unknown fields, wildcard binds (`0.0.0.0`, `[::]`), public IPs, RFC 1918 LAN
IPs, or hostnames — are refused with a clear error before the daemon starts.
Configuration error messages omit local file paths and secret material.

## Placement (witness election)

- **Empty `placement` (default):** Every node with `[metrics]` enabled emits
  both its host-local gauges (`member:<self>`) and cluster-wide gauges
  (`hev.cluster.*`, when `[cluster]` is active).
- **Non-empty `placement`:**
  - Host-local gauges (`hev.secrets.*`, `hev.orchestrator.by_state`,
    `hev.cron.failing`) are always emitted by every enabled host for itself.
  - Cluster-wide gauges (`hev.cluster.*`) are emitted only by the single
    **witness** node: the alive member listed in `placement` with the highest
    SHA-256 rendezvous hash of `"metrics\x00" + member.Name` in this node's
    SWIM view.
  - Multi-host placement requires an active `[cluster]` and waits for a seeded
    node's initial state exchange before electing a witness, matching
    [cron placement](cron.md#placement-and-cooldown). A single-entry placement
    naming this host works even without `[cluster]`.
  - When the current witness leaves or SWIM marks it dead, the next highest
    alive member in `placement` automatically takes over emitting
    `hev.cluster.*` gauges on the next tick or scrape.

## Allowlisted metrics and tags

Every emitted gauge passes through a strict central allowlist. **Secrets,
secret references, local paths, repository names, prompts, and arbitrary user
strings are never collected or emitted.**

### Cluster-wide gauges (`member:<name>,source:swim`)

Emitted once per known SWIM member when `[cluster]` is active and this host is
eligible under `placement`:

| DogStatsD name | Prometheus name | Tags | Description |
|----------------|-----------------|------|-------------|
| `hev.cluster.member_alive` | `hev_cluster_member_alive` | `member:<name>,source:swim` | `1` if SWIM state is `alive`, `0` if `dead` or `left`. |
| `hev.cluster.version_skew` | `hev_cluster_version_skew` | `member:<name>,source:swim` | `0` if the member's kit version matches the modal version among alive members (ties break toward self), `1` otherwise. |
| `hev.cluster.headless_runs` | `hev_cluster_headless_runs` | `member:<name>,source:swim` | Active headless dispatch runs reported in the member's SWIM activity summary (omitted when member is not alive or count is unknown). |
| `hev.cluster.orchestrators` | `hev_cluster_orchestrators` | `member:<name>,source:swim` | Live orchestrator supervisors reported in SWIM activity (omitted when not alive or unknown). |
| `hev.cluster.agents_working` | `hev_cluster_agents_working` | `member:<name>,source:swim` | Working agent panes reported in SWIM activity (omitted when not alive or unknown). |
| `hev.cluster.agents_idle` | `hev_cluster_agents_idle` | `member:<name>,source:swim` | Idle agent panes reported in SWIM activity (omitted when not alive or unknown). |

### Host-local gauges (`member:<self>`)

Emitted for the local node on every tick and `/metrics` scrape:

| DogStatsD name | Prometheus name | Tags | Description |
|----------------|-----------------|------|-------------|
| `hev.secrets.op_used_hourly` | `hev_secrets_op_used_hourly` | `member:<self>` | Local host secrets budget requests used in the rolling hour (from local ledger; never calls `op`). |
| `hev.secrets.op_hourly_pct` | `hev_secrets_op_hourly_pct` | `member:<self>` | Percentage of the local host's hourly secrets budget used (`0`–`100+`, rounded to 2 decimal places). |
| `hev.secrets.op_used_daily` | `hev_secrets_op_used_daily` | `member:<self>` | Local host secrets budget requests used in the rolling 24-hour window. |
| `hev.secrets.op_daily_pct` | `hev_secrets_op_daily_pct` | `member:<self>` | Percentage of the local host's daily secrets budget used (`0`–`100+`, rounded to 2 decimal places). |
| `hev.orchestrator.by_state` | `hev_orchestrator_by_state` | `member:<self>,state:<state>` | Count of local orchestrators in each allowlisted state: `running`, `waiting` (includes `ready`/`idle`), `failed`, `stalled`, and `broken`. All five series are emitted (`0` when empty); terminal/inactive states (`paused`, `stopped`, `complete`, `handed-off`) are not emitted. |
| `hev.cron.failing` | `hev_cron_failing` | `member:<self>` | Count of enabled, unpaused, locally eligible `[[cron]]` jobs with `consecutive_failures > 0`. |

## Wire formats and bounds

- **DogStatsD UDP:** Gauges are sent as newline-delimited datagrams in
  `<name>:<value>|g|#<tag1>:<val1>,<tag2>:<val2>` format, batched at most
  1024 bytes per UDP packet (fitting safely inside Tailscale's 1280-byte MTU)
  with a 2-second write deadline.
- **Prometheus `/metrics`:** Serves `text/plain; version=0.0.4; charset=utf-8`
  with `# HELP`, `# TYPE ... gauge`, and sorted metric lines on `GET` / `HEAD`
  `/metrics`. Non-`/metrics` paths return `404` and non-GET/HEAD methods return
  `405`. The HTTP server enforces bounded `ReadHeaderTimeout` (5s),
  `ReadTimeout` (10s), `WriteTimeout` (10s), `IdleTimeout` (30s), and
  `MaxHeaderBytes` (4096), and drains cleanly on `hevd` shutdown.

## Retiring external metrics scripts

If a host previously ran `hev-cluster-metrics.py` from cron or systemd:

1. Add `[metrics]` with your local or Tailscale `dogstatsd` (and/or
   `prometheus_addr`) and optional `placement` list to `~/.hev/config.toml`.
2. Restart `hevd` (`hev daemon stop && hev daemon start`, or restart its
   systemd/launchd/Windows service).
3. Verify gauges arrive at your DogStatsD agent or `curl http://127.0.0.1:9108/metrics`.
4. Disable and remove the external Python cron/timer unit.
