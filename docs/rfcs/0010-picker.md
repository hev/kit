# RFC 0010: `hev pick` — the terminal front door, in the one binary

Status: Proposed (2026-11-05). Queued behind the vocabulary cleanup that
follows RFC 0008 phase 2. Depends on RFC 0008 (orchestrators).

## Summary

Kit dispatches work, captures every session and lets agents message each
other, but the thing a person actually opens many times a day, a picker for
"which agent session do I jump to, or start", lives outside kit as a separate
Rust binary plus Python helpers. This RFC moves that picker into kit as
`hev pick`, a tmux popup over the same data `hev dispatch ls`,
`hev orchestrate ls` and `hev peers ls` already return. It ships in two steps:
make kit's native `hev peers` the only peers every host uses, then port the
picker.

## Motivation

- **The front door isn't in the box.** `brew install` gives you dispatch,
  traces, peers and orchestrators, but no way to see and switch between the
  sessions they produce. Everyone rebuilds this with `tmux choose-tree`, fzf
  one-liners or a dashboard.
- **The existing picker is two languages and five processes.** About 3,600
  lines of Rust call about 1,000 lines of Python helpers for host status,
  session data and close-out, plus `fzf`, `gum`, `ssh` and `ps`. Each helper
  re-derives state that kit already has as JSON.
- **It drifts from kit.** When the close-out path depended on a retired
  coordinator, closing sessions failed until the helper was rewritten to read
  `hev orchestrate ls --json`. A picker inside kit changes in the same commit
  as the contracts it reads.
- **Peers exists three times.** Kit has native Go peers (`internal/peers`,
  `hev peers`), but hosts still run an older Python peers tool, two copies of
  it, kept in step by hand. The picker reads peers tags, so it would inherit
  whichever one wins.

## Decision

1. **Step 1, one peers.** Bring kit's `hev peers` to parity with the features
   hosts rely on, then make it the only peers on every host. A `peers`
   symlink to `hev` already accepts the same arguments.
2. **Step 2, `hev pick`.** Port the picker to Go inside kit. It reads kit's
   own state in-process and keeps tmux as its only hard dependency.

## Goals

- `hev pick` opens a popup listing live sessions on this machine and on
  configured hosts: agents mid-turn, idle agents, dispatch runs and
  orchestrators, each with its title and state.
- Enter on a session switches to it, attaching over ssh for a remote one.
  Enter on a project opens a new session there with a chosen harness.
- Closing a session from the picker always closes it. Orchestrators whose
  `reply` is that session get a note in their inbox and a wake, as the
  current helper does.
- An agent wall: a full-screen grid of every session's last lines.
- A host footer with each host's status and active-agent count, from dispatch
  host config.
- `hev pick --install-tmux` prints, or with `--write` adds, the key bindings
  (`prefix f` picker, `prefix W` wall). Nothing is bound by default.
- Works without fzf or gum. If fzf is present, it can be used for fuzzy
  filtering.

## Non-goals

- A web or phone UI. That's dots, not kit.
- Replacing tmux. Sessions stay tmux sessions.
- Personal layout: theme tables, named reception sessions and per-client hosts
  stay in the user's config, not in kit's defaults.

## Vocabulary

- **Session:** a tmux session with an agent in it, or a dispatch run's or an
  orchestrator's session.
- **Project:** a repo the user can open a session in. The list comes from
  dispatch admission (repos a host accepts) plus `~/.config/hev/pick.toml`.
- **Role:** the session's peers role. It uses the plain names left by the
  vocabulary cleanup: operator, run, orchestrator.

## Proposed surface

```sh
hev pick                    # popup: sessions, then projects
hev pick --wall             # full-screen grid of every session
hev pick --close ID...      # the close path, also used by the popup
hev pick --install-tmux [--write]
hev pick --json             # the rows the popup renders, for other front ends
```

```toml
# ~/.config/hev/pick.toml (all optional)
hosts = ["build-1", "mac-1"]           # default: dispatch's configured hosts
projects = ["~/src/app", "~/src/site"] # added to dispatch admission
new_session = "claude"                 # or codex, pi, or a command template
harnesses = ["claude", "codex", "pi"]  # the new-session harness toggle
[[group]]                              # optional user-defined groupings
name = "client-a"
match = "client-a-*"
```

Data sources, all in-process:

| Row | Source |
|---|---|
| Local agent sessions | `internal/peers` discovery (tmux panes + harness detection) |
| Remote sessions | peers snapshots from each host (the cross-host relay in step 1) |
| Runs | dispatch inventory (`hev dispatch ls --json` contract) |
| Orchestrators | `hev orchestrate ls --json` contract, including `reply` |
| Hosts | dispatch host config and reachability |
| Projects | dispatch admission and `pick.toml` |

## Step 1: one peers

Kit's `hev peers` covers discovery, read, send and the inbox. Before hosts can
drop the Python tool, it needs what they rely on:

- **The cross-host relay:** a machine that can't be reached directly, such as
  a laptop, pushes its snapshot and collects queued messages on an interval.
  The relay's ssh calls need connect and keepalive timeouts. A hung push must
  never stop later ones.
- **Group chats** (`chat new|say|add|show|ls|leave`).
- **Inbox hooks** for harnesses that read held messages at turn boundaries,
  plus explicit acknowledgement.
- **Roles and session tags** as tmux options, with the old role names
  accepted as input aliases.
- **The send log** that front ends read.

Cut-over is per host: point `peers` at `hev`, keep the old tool's state
readable for one release, then remove it. Acceptance is the same scripted
send, read, inbox and chat round trip between three hosts on both tools.

## Step 2: the port

- Port the Rust picker's behavior, not its code: session list, project list,
  new-session harness toggle, close, wall and footer. Its existing flow tests
  become Go tests that run against fake tmux.
- The data and close helpers disappear. Their logic becomes calls into
  dispatch, orchestrate and peers.
- The status-line helper becomes `hev pick --status`, a one-line summary for
  tmux's status bar.
- Ship it in the existing release. No new binary, and the Homebrew formula is
  unchanged.

## Open questions

1. **Fuzzy matching:** a built-in matcher, or keep `fzf` as an optional
   accelerator?
2. **Remote attach:** always `ssh -t host tmux attach`, or let a host declare
   its own attach command (for example, a Windows host without tmux)?
3. **Groups:** are user-defined groups in `pick.toml` enough, or should
   orchestrators and their runs nest under each other in the list?
4. **Old picker:** keep it working against `hev pick --json` for a release,
   or remove it at cut-over?
