# RFC 0020: Headless back of house: workers and orchestrators without tmux

Status: Proposed (2026-11-17). Builds on [RFC 0008](0008-orchestrators.md)
(orchestrators), [RFC 0013](0013-hooks-and-steering.md) (hooks and
steering) and [RFC 0019](0019-gates.md) (gates, in PR #268). Evidence: the
cluster trace review (PR #270, board #514).

## Summary

tmux stays where a person uses it: front-of-house sessions that someone
attaches to, reads and types into. Back of house (dispatch workers,
orchestrator rounds and supervisors, expediters) stops using tmux on Unix.

Each BOH process gets its own process group, started by hevd under launchd
or a `systemd --user` scope. Its stream-json is written to the run's log,
and it is addressed by run name, not by pane. Windows already works this way
(job objects plus a one-shot scheduled task), so this brings Unix in line
with it.

## Motivation

BOH processes are already headless. `internal/dispatch/run.go` launches
`claude -p --output-format stream-json` or `codex exec --json`, and
orchestrator rounds do the same. Steering already reaches them without a
pane: hooks find the run through `HEV_STEER_DIR` or the run directory, and
the halt fallback is a SIGINT to the recorded harness PID
(`steer_unix.go`). Nobody attaches to these sessions.

For BOH, tmux is now only a background holder for processes, and it costs
more than it gives:

- **Shared fate.** Every run on a host lives in one tmux server. In the
  30-day trace review, Oven's server died with every worker in it five
  times. Three were a kit test escaping socket isolation, one was a worker's
  own `tmux kill-server` (`$TMUX` was set inside its pane), and one has no
  known cause. Those deaths caused 16 environment reworks across 13 runs,
  and killed `d2048-coach-resume` outright.
- **Stale environment.** A tmux server keeps the environment it started
  with. Dispatch forwards an allowlist with `tmux new-session -e`, and
  unsets plugin-only names that "a pre-existing tmux server may carry"
  (`process_unix.go`, `orchestrator_tmux_unix.go`).
- **`TMUX` leaks into work.** Workers and their tests see that they are
  inside tmux. The isolation rule
  `env -u TMUX -u TMUX_PANE TMUX_TMPDIR=$(mktemp -d)` had to be pasted into
  four runs at once.
- **Liveness is guessed from tmux.** A run is "live" if `d-NAME` appears in
  `tmux list-sessions`. The code has to tell "no server running" apart from
  "server exited unexpectedly" (`process_unix.go`). An orphaned harness
  outlives its session, and the review found runs stuck `running` after the
  harness exited.
- **A pane implies a human is watching.** Addressing a worker by pane
  invites typing into it (`peers send` pastes and presses Enter). Steering a
  headless run is a different mechanism with receipts, and the two should
  not look alike.

## Decision

1. **BOH never uses tmux on Unix.** This covers dispatch runs (today
   `d-NAME`), orchestrator supervisors and rounds (`o-NAME`), and
   expediters. FOH (sessions a person attaches to) keeps tmux and peers'
   pane features unchanged.
2. **One supervised process group per run.** hevd launches
   `hev dispatch supervise NAME` (or `hev orchestrate supervise NAME`):
   - **On macOS:** as a launchd job (`launchctl bootstrap gui/UID` with a
     generated plist, or `launchctl submit`), labelled `dev.hev.run.NAME`.
   - **On Linux:** with `systemd-run --user --scope --unit=hev-run-NAME`
     when a user manager exists, otherwise `setsid` plus a pidfd watch from
     hevd.

   The supervisor starts the harness in a new process group, as Windows
   already does with a kill-on-close job.
3. **A clean environment, built per launch.** The child environment is the
   allowlist computed from the host's config and hevd's own environment,
   exactly as `launchEnvironment` computes it today. No server environment
   is inherited, and `TMUX`/`TMUX_PANE` are never set.
4. **Liveness from the process, not a session list.** The run record holds
   the supervisor's and the harness's PID, start time and (on Linux) cgroup
   or (on macOS) launchd label. A run is live while that process is.
   Stopping a run signals its process group, or stops its unit or job.
5. **Address by name through hevd.** Peers lists BOH runs from hevd's run
   table, not from panes:
   - `hev peers read d-NAME` renders the run's stream-json log (the
     existing `renderEvents`).
   - `hev peers send d-NAME` becomes a steering note (RFC 0013), with a
     receipt.

   Nothing is pasted into a BOH process.

## Goals

- No BOH process can be killed by another run's tmux command.
- One launch, stop and liveness model on macOS, Linux and Windows.
- `peers ls`, `read`, `send` and `hev steer` keep working for BOH runs by
  name, with the same output shape.
- Workers and tests never see `TMUX`.

## Non-goals

- **Removing tmux from FOH or from peers.** People keep tmux. Peers keeps
  discovering, reading and sending to FOH panes.
- **Interactive harness modes for BOH.** BOH stays `-p` / `exec`. A person
  who wants to drive a run interactively does it from FOH with
  `hev steer` and `hev review`, or takes the branch.
- **Sandboxing.** A process group is not a sandbox (BYOS; see RFC 0019).

## Proposed surface

The commands stay the same. These change underneath:

| Today | Headless BOH |
|---|---|
| `tmux new-session -d -s d-NAME ... hev dispatch supervise NAME` | hevd: launchd job or systemd scope, `hev dispatch supervise NAME` |
| `tmux new-session -d -s o-NAME ... orchestrate supervise` | Same, `dev.hev.orch.NAME` / `hev-orch-NAME` |
| Live = `d-NAME` in `tmux list-sessions` | Live = recorded supervisor process (PID + start time + unit) |
| Env via `tmux -e` and `unset` of stale names | Env built once per launch from the allowlist |
| `peers read d-NAME` = `capture-pane` | `peers read d-NAME` = stream-json render |
| `peers send d-NAME` = paste-buffer + Enter | `peers send d-NAME` = steering note with receipt |
| `@peers-role orchestrator` pane option | Role from hevd's run table |
| `dispatch logs -f` reads the log | Unchanged |

`hev dispatch ls` gains a `SUPERVISOR` column (`launchd`, `systemd`,
`setsid`, `job`) in `--wide`. `hev s` reports which launcher the host uses.

### When hevd isn't running

Dispatch works without the capture daemon today, and must keep working.
Without hevd, `hev dispatch` launches the supervisor itself with `setsid`
and records it the same way. hevd adopts such runs from their records when
it starts. On macOS, `launchctl submit` needs no daemon.

## Rollout

1. **Liveness from processes.** Record supervisor PID, start time and unit
   for every run, and read liveness from them, with tmux as a fallback for
   runs already live. This fixes "stuck running" on its own.
2. **Headless launch behind a host setting.** Add
   `[dispatch] launcher = "auto" | "tmux"`. `auto` picks launchd, systemd or
   setsid. Turn it on for oven first, then the mini.
3. **Peers by name.** BOH runs appear from hevd's run table. `read` renders
   stream-json and `send` steers.
4. **Orchestrators and expediters** move to the same launcher.
5. **Remove the tmux launch path for BOH** one release after `auto` becomes
   the default. Keep `launcher = "tmux"` for one more release as an escape
   hatch.

## Acceptance (for the implementation)

- `tmux kill-server` run by a worker, and a test that escapes its socket,
  kill no other run on the host. Shown on oven with three concurrent runs.
- A run whose harness exits is no longer `running` within one supervisor
  tick, with or without hevd.
- `env` inside a worker shows no `TMUX` or `TMUX_PANE`, and only the
  allowlisted names.
- `peers ls` lists BOH runs, `peers read d-NAME` shows recent events, and
  `peers send d-NAME` returns a steering receipt.
- Windows behavior unchanged.

## Open questions

1. **Login sessions.** A launchd job in `gui/UID` needs a logged-in user. On
   the mini that's fine. On a headless Mac, is `user/UID` enough for
   keychain-backed harness auth?
2. **systemd linger.** Oven needs `loginctl enable-linger` for user units to
   survive logout. Should `hev up` check it, or should the launcher fall
   back to `setsid` without it?
3. **BOH interactive agents.** Some BOH agents (the mini's briefed workers,
   the `shell` session) are interactive Claude sessions in tmux today.
   Should they become dispatch runs or orchestrators, so BOH has no tmux at
   all, or stay as attended sessions?
