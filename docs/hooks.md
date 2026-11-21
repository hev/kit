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
| `prompt-submit` | `UserPromptSubmit` | add context, or halt (refuse the prompt) |
| `pre-tool` | `PreToolUse` | block the call with a reason, or halt |
| `post-tool` | `PostToolUse` | add context, or halt |
| `stop` | `Stop` | observe |
| `pre-compact` | `PreCompact` | observe |
| `needs-you`, `ready`, `done` | Kit attention notices | observe |
| `peer-message` | Kit peers delivery or chat post | observe |
| `board-post` | Kit board thread or reply | observe |
| `evidence-created`, `evidence-updated`, `evidence-sealed` | Kit [evidence](evidence.md) package written | observe |

Codex 0.162 sends Claude-compatible JSON (plus `turn_id`); its tools are `Bash`
and `apply_patch`.

pi is not supported. It is not installed on our hosts, so its extension API is
unverified and `--harness pi` is refused. Dispatch runs and orchestrators accept
only Claude Code and Codex, so there is no pi supervisor to fall back to
either. The only steering that reaches pi is a note through `peers send` to a pi
session started with `peers runtime pi` (peers' own inbox). Urgent, halt and
redirect do not reach pi.

Kit also raises three run-lifecycle events of its own, outside any harness
session: `needs-you`, `ready` and `done`. They are [attention notices](notify.md)
(`hev.attention.event/v1`, [schemas/attention_event.schema.json](../schemas/attention_event.schema.json)),
delivered to the same `hev-hook-NAME` handlers on the host that emits them,
once per notice. `needs-you` is raised only for an open decision or a halt
escalated to a hard stop, and an unchanged subject does not run handlers
again ([dedupe by subject](notify.md#dedupe-by-subject)). Each event says who it is for in `requested_by` and
`addressee` ([who a notice is for](notify.md#who-a-notice-is-for)). Handlers
observe; their output is ignored.

Kit also raises `peer-message` on the sending host after a peer message is
delivered or a chat message is posted, and `board-post` on the CLI host after
the board accepts a thread or reply. See the [peers event schema](../schemas/peer_message_event.schema.json)
and [board event schema](../schemas/board_post_event.schema.json). The same
observe-only delivery applies. The message or body is redacted before handlers
see it. On a client host (`[notify] client = true`) handlers get metadata and
byte counts with the content field omitted. The copy in `events.jsonl` omits
the content field on every host.

Writing an [evidence package](evidence.md) raises `evidence-created`,
`evidence-updated` and `evidence-sealed` on the writing host, observe-only
like the events above ([evidence event schema](../schemas/evidence_event.schema.json)).
The payload is package and item metadata and claims that were redacted before
they reached the disk; it never carries item bytes or text. `evidence-*` names
the family.

## Fail open, always

A broken kit must never stop an agent.

- Every handler runs under one budget, `[hooks] timeout` (default `2s`).
  A handler that errors, panics, exits non-zero, prints invalid JSON or misses
  the budget is dropped, and a warning goes to `~/.hev/hooks/warnings.log`.
  Its process group is killed before the hook exits.
- `hev hook` always exits 0 (a Claude hook exiting 2 would block). A watchdog
  ends the process at the budget plus 3 seconds whatever it is doing. It starts
  before the config file is read (with the default budget until the configured
  one is known), and it exits after at most 200ms of trying to log its warning,
  so a blocked log cannot keep the process alive.
- A config file or hook log that is not a regular file (a FIFO, a device) is
  refused with a warning rather than opened, because opening it can block.
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
- `feed` spools the event for the [Slack feed](feed.md) when `[feed]` is
  configured and wakes its sender; it adds nothing to the answer. The same
  feed also observes `needs-you`, `ready`, `done`, `peer-message` and
  `board-post`.
- `peers-inbox` injects the bound peers inbox on `prompt-submit`, exactly as
  `hev peers inbox-hook` does, in sessions `peers runtime` bound
  (`PEERS_INBOX_TOKEN`). A `peers runtime` launch wires `inbox-hook` itself and
  sets `PEERS_INBOX_HOOKED=1`, so the handler steps aside there.

User handlers are executables named `hev-hook-NAME` on PATH, found like
[exec commands](plugins.md) (first match per name, `.exe`/`.cmd` on Windows). In a
`go test` binary only handlers under the temp directory are found, so tests
never run a developer's installed handlers. Each
gets the event name as its argument, the full event JSON on stdin, and
`HEV_HOOK_EVENT` and `HEV_HOOK_HARNESS` in its environment (`kit` for the
Kit events, whose stdin is the event-specific JSON). Harness handlers print nothing
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

## Hosted event forwarding

When the host has a Kit API key, `hev hook` also posts supported Claude Code
and Codex events to `/v1/hooks/events`. The hosted answer is merged with the
local handlers under the same hook deadline. A failed request produces a
warning and local handlers still decide the result. The per-host
`events.jsonl` continues to record the redacted event and merged decision.

Hosted webhook registrations accept public HTTPS endpoints. Deliveries use
`X-Hev-Signature-256` with an HMAC-SHA256 over the Unix timestamp, a period,
and the exact request body. Verify the timestamp and signature before using
the payload. Observe deliveries can be repeated under the same delivery ID.
Blocking deliveries run once and fail open on errors or timeout.

Hosted event IDs replay the original blocking decision and optional text while
the event is retained. The event history keeps up to 30 days and evicts the
oldest completed rows when an account reaches its row limit. Webhook responses
can supply 10,000 characters per text field. When several blocking receivers
answer, the merged text fields each keep their first 10,000 characters and
decision precedence still applies.

Observe jobs for content-free payloads resume after a hosted restart. Hosted
board jobs with content enabled recover their payload from the committed board
post. Opted-in harness content exists only in memory. If the hosted process
restarts before it finishes delivering that content, the delivery is marked
failed because the original payload is unavailable.
