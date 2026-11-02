# Agent peers

`hev peers` is native Go. It discovers agent processes beneath local tmux panes
on Linux and macOS and includes dispatch runs from `~/dispatch/*/meta.json`.
No external `peers` program is required. `HEV_PEERS_BACKEND`, when explicitly set,
selects an external executable instead. `list` and `show` alias `ls` and `read`.
A symlink or copy of the executable named `peers` accepts the same arguments
without the `hev peers` prefix.

```sh
hev peers ls --json
hev peers ls -a                 # include plain shells
hev peers read desk -n 120
hev peers send desk 'Review is ready' --from reviewer
hev peers send host/desk - < message.txt
```

Addresses are `NAME`, `HOST/NAME`, or a local pane ID. Ambiguous session names
are refused; select a pane. `PEERS_SELF` sets this machine's stable host label;
the default is its hostname before the first dot. Set the same label on all
invocations, including remote endpoints, and preserve it during migration.
`--from` or `PEERS_FROM` supplies sender provenance when automatic pane/process
ancestry detection is unavailable. Neither grants inbox consumption rights.

Interactive delivery uses a tmux buffer and bracketed paste. Recognized harnesses
include Claude Code, Codex, Gemini, OpenCode and PI. Unknown harnesses can register
an inbox. Plain shells and detected headless invocations require inbox delivery.
Visible permission dialogs and drafts are refused. Unrecognized screens require
`--force`; PI always requires an inbox. Screen interpretation is deliberately
conservative: inboxes are the reliable option for new harness versions. Discovery,
reads and inbox operations never wake an agent.

## Inbox

Register from the recipient pane, or explicitly name its session for setup:

```sh
hev peers inbox on [SESSION]
hev peers inbox off [SESSION]
hev peers inbox --peek --pending --json
hev peers inbox --count
hev peers inbox consume FULL_ID...
hev peers inbox ack FULL_ID...
hev peers inbox hold --sender another-agent - < message.txt
```

Registered inboxes hold messages instead of typing into the terminal, even during
a draft or permission prompt. A send reports the full message ID and that receipt
has not been acknowledged. `send --id RETRY_KEY` deduplicates held messages;
reusing a key with changed content, sender or recipient lifetime is refused.
A retry remains held even if inbox registration was subsequently turned off.
Interactive pastes and run markdown appends are not deduplicated by `--id`.

`--peek` observes without consuming. `--count` reports unread messages. Explicit
`consume ID...` validates every ID before recording any consumption, and repeated
consumption is harmless. The default `inbox --json` consumes its returned batch.
`--pending` replays every unacknowledged item, including consumed items, until
explicit ACK. `--limit 1..64` bounds a batch (default 64). Only consumption reduces
the unread count; only ACK removes an item from pending. ACK requires prior
consumption of every ID in the batch; an unknown or unread ID refuses the whole
batch. Repeated ACK is harmless. Inspection and receipt do not imply handling.

Consumption and ACK require the actual current recipient pane/process; selecting
`--session` or `--from` cannot impersonate it. `--session NAME --peek` permits
inspection. `--all --json --session NAME` inspects retained history, including a
dead session's history. `--current-all` inspects current-lifetime history.
`inbox owner --json` returns the current recipient binding and registration.
This is a same-user tmux contract, not isolation from another process that can
access the user's socket and files. Detached runtime token delegation is outside
this implementation; consumers must retain their pane or process ancestry.

### Storage and consumer compatibility

State defaults to `~/.local/state/peers-v2`; `HEV_PEERS_STATE` selects an isolated
state root. Messages retain `inbox/SESSION.jsonl`, sanitized to letters, numbers,
dots, underscores and hyphens. Consumption and ACK append to
`SESSION.jsonl.receipts.jsonl` under the same `SESSION.jsonl.lock` flock on Unix
(and native file locking on Windows). Writes are synced before reporting success.
Existing bytes, including torn tails, legacy rows and numeric `.cursor` files,
are preserved. Invalid rows are ignored; complete rows without a trailing newline
remain readable. New writes separate an incomplete tail before appending.

The JSONL message fields are `id`, numeric `ts`, `from`, `header`, `message`,
`recipient_session`, `recipient_incarnation`, and `transport_state`. The incarnation
uses the host label, tmux server PID/start time and session ID/creation time;
recreating a session never adopts its predecessor's obligations. Existing bound
rows and ID/incarnation receipts remain usable with the same host label. Unbound
legacy rows remain historical; they are never automatically replayed.

CLI JSON returns `session`, `recipient_incarnation`, `on`, `unread`,
`unacknowledged`, `historical`, and `messages`. Each message also has computed
`current`, `consumed_at`, `acknowledged_at`, and `transport_state`. Receipt rows
retain `id`, `recipient_incarnation`, `kind` (`consumed` or `acknowledged`) and `ts`.
The consumer command shapes `inbox --peek --pending --json`, `inbox consume ID...`,
`inbox ack ID...`, `inbox --pending --limit 8 --json`, and `inbox --count` are tested
as a compatibility contract. No cursor rewrites or synthetic ACKs occur.

Stop old consumers/writers before switching executables. Inspect retained history
and verify host labels and bindings first; do not point an older cursor-based
consumer at newer receipt state or bulk replay unbound history.

## Dispatch runs

A dispatch run is addressed by `HOST/ORDER-NAME`, independent of the transient
`d-ORDER-NAME` tmux session, harness PID, restart or rework round. Discovery collapses
that transient session into the run row. `hev peers read ORDER-NAME` reads its log.

A send to a live run appends to its `inbox.md`, under the dispatch lifecycle lock,
and reports **delivered to run inbox**. A send to an ended run appends there and
reports **held in ended run inbox; no rework started**. This path never calls
rework. An explicit later rework archives the inbox with other round artifacts;
held messages remain in that archive for inspection rather than being silently
submitted to a new round. `hev dispatch say` retains its existing semantics.
The run markdown inbox and the terminal JSONL inbox are separate formats; use
`dispatch show` to inspect run markdown.

Run processes can send using their stable identity from their worktree. Dispatch
notification commands receive `PEERS_FROM=HOST/ORDER-NAME` (or `ORDER-NAME` when
no host label is recorded), so a reply destination remains usable after the
sender's terminal ends. Configure a notification command using the native CLI,
for example `['hev', 'peers', 'send', '{reply}', '{name}: {state} {summary}']`.

Windows supports discovery, messaging and log reads for dispatch runs, including
held ended-run delivery, without tmux. Interactive terminal discovery, capture,
paste and terminal inbox registration require tmux on Linux or macOS. This phase
does not supply a Windows terminal adapter.

## Remote transport and phase 2

`HOST/NAME` uses SSH to run `hev peers` on the destination. Both endpoints need a
compatible native binary on PATH and matching host-label configuration. Host names
use SSH configuration aliases; normal SSH host-key verification applies. No remote
shell interpolation of message text occurs: arguments are quoted and stdin stays
stdin. `ls --everywhere` visits hosts listed in whitespace-separated `PEERS_HOSTS`.
With `--json`, it emits one inventory JSON object per host (JSON Lines).
Remote inbox access is inspection-only (`--peek`, `--count`, `--all` or
`--current-all`). SSH errors are visible; a lost response can leave delivery
uncertain. Inspect before retrying, and reuse `--id` for JSONL inbox sends. There
is no offline SSH relay, automatic retry or automatic reverse ACK delivery.

All remote operations pass through the `Transport.Execute` interface. Phase 2
can implement a hevd transport without changing command parsing or local inbox
semantics:

1. Define versioned inventory, read, enqueue and receipt-inspection RPCs with
   bounded responses and authenticated host/sender provenance.
2. Carry full message IDs and intended recipient incarnations across the wire;
   preserve conflicts, consumed/unacknowledged replay and explicit ACK ownership.
3. Add durable outbound transfer receipts and retry keys so a lost response or
   daemon restart cannot drop or duplicate an obligation. Specify run markdown
   deduplication before enabling automatic retries for run delivery.
4. Qualify mixed versions, host/session recreation, SSH fallback, interrupted
   transfers and rollback against the existing CLI/JSON compatibility suite.
5. Make transport selection explicit, preserving SSH as a fallback until daemon
   capability negotiation and failure recovery have passed live acceptance.

Group chats, idle wake, and hevd transport are outside phase 1.
