# Agent peers

`hev peers` is native Go. It discovers agent processes beneath local tmux panes
on Linux and macOS and includes dispatch runs from `~/dispatch/*/meta.json`.
No external `peers` program is required. `HEV_PEERS_BACKEND`, when explicitly set,
selects an external executable instead when invoked as `hev peers`. The `peers`
alias ignores this override to prevent recursive forwarding. Help is always local. `list` and `show` alias `ls` and `read`.
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
explicit ACK. `--limit N` bounds a batch when given; the default is unlimited. Only consumption reduces
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

Set `PEERS_SELF` explicitly on macOS hosts before cutover: the Python helper
lowercases and maps host names, while native discovery preserves the hostname.
Use the existing canonical label so recipient bindings remain valid.

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

## Remote transport

`HOST/NAME` uses the `Transport.Execute` boundary. SSH remains the default for
all operations. Both endpoints need a compatible native binary and stable host
labels. SSH uses configured aliases and normal host-key verification; message
text is passed through stdin without shell interpolation. `ls --everywhere`
visits whitespace-separated `PEERS_HOSTS`. Remote inbox access remains inspection
only; consumption and ACK require local recipient ownership.

### hevd setup and security

hevd has an opt-in authenticated enqueue listener. Add to the host's
`~/.hev/config.toml` (or `HEV_CONFIG`), keeping that file mode 0600:

```toml
[peers]
listen = "127.0.0.1:8789"
token = "REPLACE_WITH_RANDOM_SECRET_AT_LEAST_32_CHARACTERS"

[peers.hosts.workstation]
url = "https://peers.example.test"
token = "REPLACE_WITH_DESTINATION_SECRET"
```

Generate a random token locally, store it directly in configuration, and share it
only with authorized sending hosts. Tokens are never included in diagnostics.
Normal `hev daemon` starts the listener alongside capture. For a messaging-only
host, run `hev daemon --peers-only`; this mode does not require Layer credentials
or start archival work. `listen` must be set in this mode. Stop it with SIGTERM
or Ctrl-C. Omit `listen` to disable inbound delivery while retaining outbound
configuration. In normal capture mode an invalid peers section or listener bind
failure logs one warning and disables the optional endpoint; capture continues.
Remote SSH operations warn and fall back to SSH if peers configuration is invalid.
The explicit messaging-only mode refuses invalid configuration.

For sessions, register the destination with `hev peers inbox on SESSION` first.
Dispatch runs already have an inbox. HTTP never pastes into a terminal and cannot
consume messages, ACK receipts, start runs, or request rework. The endpoint is
`POST /v1/peers/enqueue`, with bearer authentication and a bounded JSON object:
`target`, `from`, `message`, optional `id`. The sender must be `HOST/NAME`,
with exactly one slash and two non-empty parts of at most 255 ASCII characters
each. Parts allow letters (case preserved), digits, hyphens, underscores and
periods; a leading hyphen, controls and whitespace are rejected. IDs cannot
contain control characters. The response contains `status` and
`detail`. Requests and responses are limited to 1 MiB on the client; the server
also limits request size and applies connection timeouts. Redirects are refused.

The bearer token grants enqueue access to this user's registered local inboxes.
Sender names are asserted provenance by trusted token holders, not independently
authenticated identities. Use a dedicated secret per destination and rotate it
by updating that destination and its senders. Keep the listener on loopback
behind an authenticated network/tunnel or TLS reverse proxy; plain HTTP on an
untrusted network exposes tokens and message contents. Public listeners require
TLS termination and network access restrictions. No secrets belong in git.

### Selection, statuses, and failure recovery

Configure `[peers.hosts.HOST]` on the sender to prefer hevd for sends to that
host. Successful sends print `via hevd` and one of: **delivered to session inbox**,
**delivered to run inbox**, **held for ended run**, or **queued for next round**.
The current server delivers synchronously and does not emit the queued status;
the client supports it for compatible future servers. Failed enqueue responses
report **failed** with a reason. Session receipt remains unacknowledged until the
recipient consumes and ACKs it. Ended-run messages remain held without rework.
SSH sends identify `using ssh` and retain their existing delivery status.

A connection failure before submission falls back to SSH and says so. Auth
failures, HTTP errors, malformed responses, timeouts and lost responses do not
fall back: delivery may already have occurred. Inspect before retrying. Reuse
`--id` to deduplicate session inbox retries; dispatch markdown delivery has no
retry deduplication, so never automatically resend an uncertain run delivery.
No durable outbound queue, automatic retries, reverse ACK forwarding or beacon
is provided. An unreachable SSH fallback fails visibly.

An outbound-only host (including Windows behind NAT) can send directly to any
reachable destination hevd without running its own listener or accepting inbound
SSH. Reachability is directional: the receiver cannot reply through that same
HTTP connection. Give each desired recipient a reachable TLS endpoint or tunnel;
otherwise replies need SSH reachability or a later relay implementation. Windows
supports dispatch inbox delivery; terminal inbox discovery still requires tmux
on Linux or macOS.

### Next step: group chats and durable transfer

Group chats are deferred. Add versioned start/join/send operations, stable thread
IDs and host-qualified membership, per-member delivery receipts, and explicit
partial-failure handling. Before automatic retry or relay, define dispatch inbox
deduplication and durable outbound receipts, including interrupted transfers and
recipient recreation. Inventory/read/receipt inspection still use SSH. The old
helper's cached snapshots, relay/outbox, wait and spawn commands are not supplied.
