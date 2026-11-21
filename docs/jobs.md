# Scheduled jobs

Add one or more `[[jobs]]` entries to `~/.hev/config.toml` (or the file selected by
`HEV_CONFIG`). Restart the full `hevd` daemon to pick up changes.

```toml
[[jobs]]
name = "refresh-index"
every = "15m"
cmd = ["refresh-index", "--incremental"]
workdir = "~/workspace"
timeout = "2m"
cooldown = "5m"
env_refs = { INDEX_TOKEN = "secret://index/token" }

[[jobs]]
name = "daily-summary"
at = "09:30"
tz = "America/New_York"
cmd = ["daily-summary"]
placement = ["node-a", "node-b"]
enabled = true
```

Configure up to 128 jobs. Names are unique, fit in 128 bytes and match `^[a-z0-9][a-z0-9-]*$`. Set either `every` (a Go
duration of at least 10 seconds, aligned to the Unix epoch) or `at` (24-hour
`HH:MM`, with an optional IANA `tz` that defaults to UTC). During clock
changes, a missing wall-clock minute is skipped and a repeated minute runs at
its first occurrence. The daemon skips missed ticks. On restart,
scheduling resumes at the next tick.

Supply executable and arguments in `cmd`. hevd passes them directly to the
program. The default working directory is the user's home. A leading `~/` or
a bare `~` expands to that directory. Timeout defaults to five minutes and must
be positive and at most 24 hours. It covers secret resolution and execution.
The daemon cancels and waits for its running jobs on shutdown. Children must
stay in their Unix process group or Windows Job Object.

`env_refs` maps environment names to references resolved by the secrets cache
at run time. The jobs directory is private to the daemon user. Run output
scrubs stdout and stderr separately, replacing resolved values with `[REDACTED]`, applies
kit's gitleaks scrubber and removes known local paths before joining the streams. Oversized captures are
discarded to avoid persisting partial secrets at the capture boundary.

State lives in `~/.hev/jobs/<name>/state.json`, with the newest 20 entries under
`runs/`. Each entry holds at most 64 KiB of output. A tick that overlaps a
running job records `skipped_busy`. By default a job is enabled. Set
`enabled = false` to turn it off in config. Pause and resume persist locally
across daemon restarts. A success resets the failure streak. On the second
consecutive failure, jobs sends one `done` attention event with state `failed`
through the existing lifecycle hooks and outbox. Enable `done` in Slack kinds
to receive it there. The feed consumes the same lifecycle event.

## Placement and cooldown

An empty placement runs locally. With SWIM active, members rank the alive
nodes in the placement by rendezvous hash of job and member name. Only the
highest-ranked member runs the job. When the member leaves or SWIM marks it
dead, the next alive member takes over at the next eligible tick. Configure
the same job and placement on the participating hosts. Multi-host placement
requires an active cluster. A node configured with seeds waits for its first
cluster state exchange before it can run placed jobs. A singleton placement can operate without SWIM
when the local node name matches.

Cooldown is a nonnegative Go duration from the start of the last attempt.
The local start time survives restarts. Placed jobs gossip cooldown deadlines
so the next winner can honor the previous winner's cooldown. This uses each
member's current SWIM view and received facts. During partitions, divergent
views or a crash before a fact propagates, duplicate runs are possible. Use
idempotent commands for jobs that move between hosts.

## Commands

- `hev jobs ls [--json]` lists schedule, status, last exit/time and next tick.
- `hev jobs show <name> [--json]` shows state and recent redacted output.
- `hev jobs run <name>` executes immediately if enabled, unpaused, eligible for
  placement, outside cooldown and idle. A job with secret references needs the
  local daemon's secrets endpoint.
- `hev jobs pause <name>` and `hev jobs resume <name>` change local scheduling
  state. Pausing leaves a running execution to finish.
