# Services

Add one or more `[[service]]` entries to `~/.hev/config.toml` (or the file
selected by `HEV_CONFIG`) to supervise long-lived processes inside `hevd`
across macOS, Linux, and Windows without separate `launchd` plists,
`systemd --user` units, or Windows Scheduled Tasks. Restart `hevd` after
editing `[[service]]` definitions.

```toml
[[service]]
name = "board"
cmd = ["hev", "board", "serve", "--bind", "127.0.0.1:8091"]
workdir = "~/workspace"
restart = "always"
stop_timeout = "15s"
env_refs = { HEVLAYER_API_KEY = "secret://layer-api-key" }

[[service]]
name = "intranet"
cmd = ["intranet-server", "--listen", "100.64.0.10:8080"]
workdir = "~/workspace/intranet"
restart = "on-failure"
placement = ["node-a", "node-b"]
enabled = true
```

## Configuration

Configure up to 64 services. Unknown fields in `[[service]]` and invalid
settings are refused when `hevd` or `hev service` loads the file.

| Field | Default | Rule |
| --- | --- | --- |
| `name` | required | Unique service name matching `^[a-z0-9][a-z0-9-]*$` (up to 128 bytes). |
| `cmd` | required | Non-empty `[]string` argv executed directly via `exec.CommandContext` with no shell expansion. |
| `workdir` | `~` | Working directory. Bare `~` or leading `~/` expands to the user's home directory; relative paths resolve under home. |
| `env_refs` | `{}` | Map of environment variable names (`^[A-Za-z_][A-Za-z0-9_]*$`, up to 64 entries) to `secret://`, `op://`, or `azkv://` references resolved at start and every restart via `internal/secrets`. |
| `restart` | `"always"` | `"always"`, `"on-failure"` (restarts only on non-zero exit or startup failure), or `"never"`. |
| `stop_timeout` | `"15s"` | Positive Go duration up to `"5m"` before graceful termination escalates to force kill. |
| `placement` | `[]` | Optional list of distinct SWIM member names (up to 64). Empty runs locally; non-empty runs only on the active-standby rendezvous winner. |
| `enabled` | `true` | Set `false` to keep a service stopped in configuration. |

## Supervision, shutdown, and crash-loop notices

`hevd` starts enabled services automatically when local or elected by
`placement`. Each child starts in its own Unix process group (`Setpgid`) or
Windows Job Object (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`). When a service is
stopped, loses placement, or `hevd` shuts down, the supervisor signals the
process group (`SIGTERM` on Unix, console break / Job Object on Windows), waits
up to `stop_timeout`, and then terminates any remaining processes in the group
or Job Object so no orphaned children survive.

When a service exits and its `restart` policy calls for another run, the
supervisor waits in `backoff` using bounded exponential backoff starting at
`2s` and doubling up to a `5m` cap (`2s`, `4s`, `8s`, …, `5m`). Once an
instance stays up for more than `60s`, its backoff streak and crash-loop window
reset.

If a service exits rapidly (uptime `<= 60s`) three or more times within `2m`,
`hevd` emits one deduplicated `done` attention notice (`source = "service"`,
`state = "failed"`) through the lifecycle hooks, `[feed]`, and `[notify.slack]`
(when `"done"` is enabled in `kinds`). No further notice is emitted during the
same crash loop; healthy uptime (`> 60s`) resets the detector.

## State and redacted logs

Runtime files live in `~/.hev/services/<name>/` with private permissions:

- `state.json` records `status` (`running`, `stopped`, `backoff`, `standby`, or
  `failed`), `pid`, `start_time`, `last_stop`, `restarts`, `last_exit`,
  `next_restart`, and `placement_winner`.
- `service.log` is a bounded ring log capped at `1 MiB`. When output exceeds
  `1 MiB`, older lines are compacted atomically so the file never grows past
  `1 MiB`. Stdout and stderr are scrubbed separately before reaching disk:
  resolved `env_refs` secret values and known local paths are replaced with
  `[REDACTED]`, and `internal/redact` scrubs gitleaks and credential patterns.
  Single lines exceeding `64 KiB` without a newline are omitted so secrets
  cannot split across buffer boundaries.

## Active-standby SWIM placement

With `placement = []` (the default), the service runs on the local host. With
`placement = ["node-a", "node-b"]`, each node with an active `[cluster]` ranks
the `alive` SWIM members in `placement` by SHA-256 rendezvous hash of the
service name and member name:

- Only the highest-ranked alive member starts and supervises the service.
- Non-winning members and nodes outside `placement` stay in `standby` (recording
  `placement_winner` in `state.json`) and never start the service.
- When the winning node stops or leaves SWIM, the surviving highest-ranked
  member starts the service automatically; when a higher-ranked member joins,
  the previous runner stops its child process group cleanly and returns to
  `standby`.
- Multi-host `placement` fails closed (`standby`) when `[cluster]` is not
  active or before a seeded node completes its first cluster state exchange. A
  single-host `placement = ["node-a"]` can run without SWIM when the local node
  name matches.

## Commands

- `hev service ls [--json]` lists configured services with status, PID,
  restarts, last exit, start time, and placement winner.
- `hev service show <name> [--json]` inspects state, metadata, and recent
  redacted log lines.
- `hev service logs <name> [-n LINES]` prints the last `LINES` lines (default
  `100`, `0` for all retained lines) from the redacted `service.log`.
- `hev service start <name>`, `hev service stop <name>`, and
  `hev service restart <name>` ask the running `hevd` supervisor to start,
  gracefully stop, or restart a configured service and wait for completion.
  Stopping a service persists locally until `start` or `restart`.
