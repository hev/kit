# Hooks

Kit has one hook layer for Claude Code and Codex ([RFC 0013](rfcs/0013-hooks-and-steering.md)).
Every hook a harness runs is the same command:

```sh
hev hook EVENT --harness claude|codex
```

The harness passes its hook JSON on stdin. Kit normalizes it to one event
(`hev.hook.event/v1`, [schemas/hook_event.schema.json](../schemas/hook_event.schema.json)),
appends a redacted copy to `~/.hev/hooks/events.jsonl`, runs handlers, and
answers in the harness's own shape.

| Event | Claude Code / Codex name | Handlers can |
| --- | --- | --- |
| `session-start` | `SessionStart` | add context |
| `prompt-submit` | `UserPromptSubmit` | add context |
| `pre-tool` | `PreToolUse` | block the call with a reason, or halt |
| `post-tool` | `PostToolUse` | add context, or halt |
| `stop` | `Stop` | observe |
| `pre-compact` | `PreCompact` | observe |

Codex 0.162 sends Claude-compatible JSON (plus `turn_id`); its tools are `Bash`
and `apply_patch`. pi is not installed on our hosts and has no verified hook
mechanism, so `--harness pi` is refused and pi gets steering's turn-boundary
fallback.

## Fail open, always

A broken kit must never stop an agent.

- Every handler runs under one budget, `[hooks] timeout` (default `2s`).
  A handler that errors, panics, exits non-zero, prints invalid JSON or misses
  the budget is dropped, and a warning goes to `~/.hev/hooks/warnings.log`.
  Its process group is killed before the hook exits.
- `hev hook` always exits 0 (a Claude hook exiting 2 would block). A watchdog
  ends the process at the budget plus 3 seconds whatever it is doing.
- Unknown events, unknown harnesses and unparseable input answer "continue".
- The harness entry carries its own `timeout` (10 seconds); a harness treats a
  timed-out hook as a non-blocking error.

Kill switch, checked before reading stdin:

```sh
HEV_HOOKS=off                    # environment: off, 0, false or no
```

```toml
# ~/.hev/config.toml
[hooks]
enabled = false                  # default true
timeout = "2s"                   # handler budget per event
halt_timeout = "10m"             # see steering.md
```

## Handlers

Built in:

- `steer` resolves the steering target (a dispatch run, an orchestrator or a
  local peers session) and delivers [steering](steering.md).
- `peers-inbox` injects the bound peers inbox on `prompt-submit`, exactly as
  `hev peers inbox-hook` does, in sessions `peers runtime` bound
  (`PEERS_INBOX_TOKEN`). A `peers runtime` launch wires `inbox-hook` itself and
  sets `PEERS_INBOX_HOOKED=1`, so the handler steps aside there.

User handlers are executables named `hev-hook-NAME` on PATH, found like
[plugins](plugins.md) (first match per name, `.exe`/`.cmd` on Windows). Each
gets the event name as its argument, the full event JSON on stdin, and
`HEV_HOOK_EVENT` and `HEV_HOOK_HARNESS` in its environment. It prints nothing
(continue) or one JSON object:

```json
{"decision": "continue|block|halt", "reason": "shown to the model", "context": "injected", "system_message": "shown to the person"}
```

Handlers run concurrently. Merged, halt beats block beats continue, and
contexts join in handler order. For example, a handler that refuses force
pushes:

```sh
#!/bin/sh
# hev-hook-no-force-push
case "$(cat)" in
  *'"kind":"mutate"'*'push --force'*) echo '{"decision":"block","reason":"No force pushes here."}' ;;
esac
```

## Install and repair

Dispatch runs and orchestrator rounds do not need anything installed: their
supervisor wires the hooks for that one launch (Claude `--settings`, Codex
`-c hooks.EVENT=...` with `--dangerously-bypass-hook-trust`, since kit vouches
for its own command) and sets `HEV_STEER_DIR` so the hook finds the run.
Interactive sessions need the hooks in the harness's settings:

```sh
hev hooks install                # every harness home that exists
hev hooks install --harness claude
hev hooks check                  # non-zero when missing or stale
hev hooks uninstall              # removes only kit's entries
```

- Claude Code: `~/.claude/settings.json`. Codex: `~/.codex/hooks.json`
  (`$CODEX_HOME`).
- Install merges. Other hooks and settings are kept, text is not re-escaped,
  the file mode is kept, and the previous file is saved as
  `settings.json.before-hev-hooks`. Each kit command ends in
  `--installed-by kit`; `check` and `uninstall` only touch commands with that
  stamp. A second install changes nothing; an install from a new binary repairs
  kit's entries in place.
- When a run's invocation hooks and the installed hooks would both fire, the
  installed copy sees `HEV_HOOK_INVOCATION=1` and exits, so nothing is handled
  twice.
- Codex runs a global hook only after a person trusts it: open `codex` and
  review it with `/hooks`. Kit never writes that trust record. In `codex exec`,
  0.162 also skipped an untrusted invocation-level `-c hooks...` entry unless
  the launch passed `--dangerously-bypass-hook-trust` (verified on oven); the
  `UserPromptSubmit` inbox hook `peers runtime codex` wires is such an entry.
- `hev skills sync` and `hev update` repair kit's entries where they are already
  installed. They never install hooks nobody asked for.

## The event log

`~/.hev/hooks/events.jsonl` has one line per event: the normalized event
without prompt text, tool input, command or tool output (their sizes only),
plus the decision, injected bytes and elapsed milliseconds. It rotates to
`events.jsonl.1` past 32 MiB. Steering latency and blocks can be read from it
and from the receipts; `hev traces` does not ingest it yet.
