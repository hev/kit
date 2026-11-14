# RFC 0017: notify — named notifiers on a node, so orchestrators and runs can post to Slack, Teams or Discord

Status: Proposed (design only; no code, schema or command in this change).

## Summary

A node can declare **notifiers**: named chat destinations (Slack, Teams,
Discord, or a local command) with a credential reference and the channels each
may post to. An orchestrator spec, or a plain run's existing `notify` command,
picks a notifier and channel by name. kit composes every message itself from
run and orchestrator state, scrubs it with kit's redaction, and delivers it
through the durable notice queue orchestrators already use. Posting is
outbound only. A failed post is recorded on a receipt and never changes a run.

Notifiers live in a new per-node file, `notify.toml`, next to the target's
default `dispatch.toml`. The `notify` argv array in `dispatch.toml` is left
exactly as it is.

## Motivation

- **People live in chat, not in peers inboxes.** An orchestrator's owner
  notices reach a peers session (`docs/orchestrators.md`, "Owner notices").
  That only works for someone at a terminal with a registered inbox. A
  needs-decision that waits overnight because nobody read a tmux pane is the
  most expensive kind of wait.
- **Client work needs its own channel and its own app.** A node used for a
  client should post to that client's workspace with that client's app, and
  must not be able to post anywhere else. Per-node config gives that
  boundary for free: the node only holds the client's notifier.
- **Today's only hook is a run-end command.** `dispatch.toml`'s `notify` is
  one argv executed when a run ends or gets a verdict
  (`internal/dispatch/run.go`, `Notify`/`notify`). It is not durable, not
  retried, carries the agent's own marker text in `{summary}`, and is not
  available to orchestrators at all.

## Decision

1. Notifiers are named tables in a new per-node file, `notify.toml`, read only
   by the node that posts; `dispatch.toml` and its `notify` argv are unchanged.
2. A spec or command names a notifier and a channel **label**; the node's
   file maps labels to real channels, so a sender can only reach channels the
   node listed.
3. Chat posts are another route on the existing notice queue: same receipts,
   leases, backoff and terminal states, with a `notify:NOTIFIER/CHANNEL`
   recipient.
4. Messages come from fixed, code-owned templates over run state. Agent-written
   fields (decision question, block reason) are cleaned, scrubbed, capped, and
   withheld entirely from notifiers marked `audience = "external"`.
5. Phase 1 is Slack outbound on one node; Teams, Discord, plain runs and
   checks follow as separate orders.

## Decided already

These came with the order and are not reopened here.

1. The name is `notify`, not chat (chat would be confused with peers).
2. Config is per node. There is no cluster-wide config. A node with no
   notifier cannot post, and that is correct.
3. Outbound only in v1. Nobody answers, resolves, merges or steers from the
   chat app. See [Later: inbound](#later-inbound).
4. kit composes the message from run state, through kit's redaction, never
   from an agent's free prose. A client channel must not receive text nobody
   approved.

## Goals

- One node, one file, several notifiers (the operator's own workspace and a
  client's), each limited to the channels it lists.
- No existing host config breaks, including on nodes that never upgrade.
- One delivery path: the orchestrator notice queue, reused for chat.
- Messages built only from state kit owns, with a fixed template per event.
- Credentials resolved at use time through `hev secrets`; never in a config
  value, a log, a receipt or an error.
- An operator can test a notifier from the shell without posting
  (`hev notify check`) and post a fixed test message (`hev notify post --test`).

## Non-goals

- Inbound commands, buttons, slash commands or approvals from chat (v1).
- A cluster-wide notifier registry, or one node posting through another's.
- User-written message templates.
- Forwarding agent prose: `reply.md`, checkpoints, reports, logs, PR titles
  and bodies, commit messages.
- Rich formatting (Block Kit, Adaptive Cards, embeds) in v1. Plain text only.
- A daemon. Like orchestrators, nothing runs on a node that has not opted in.

## Vocabulary

- **Notifier**: a named, node-local chat destination: kind, credential
  reference, audience and channel map.
- **Kind**: `slack`, `teams`, `discord` or `command`.
- **Channel label**: the node-local name for one destination in a notifier's
  channel map (for example `builds`). Specs and commands use labels, never
  channel IDs or webhook URLs.
- **Audience**: `internal` (the operator's own people) or `external` (a
  client or anyone else who must not see unapproved text).
- **Chat route**: a notice receipt whose recipient is `notify:NOTIFIER/LABEL`.
- **Thread anchor**: the message reference of the first post for an
  orchestrator lifetime or a run, which later posts reply under.

## What kit already has

Every row cites a file on `main` this design was checked against.

| Piece | Where | Used here as |
| --- | --- | --- |
| `notify` argv: one command, placeholders `{requested_by}`, `{sender}`, `{name}`, `{state}`, `{summary}`, `{reply}`, run without a shell | `internal/dispatch/run.go` (`notify`, `executeNotification`) | Unchanged; can call `hev notify post --run {name}` |
| Run-end notify is synchronous, 20 s, failure only sets `notify_error` | `internal/dispatch/run.go` (`notifyCtx`, `setNotifyError`); `docs/dispatch.md` | The guarantee kept: a post never changes run state |
| `{summary}` begins with the worker's marker text (`blocked: REASON`, `needs-decision KEY: QUESTION`) | `internal/dispatch/run.go` (`noticeSummary`), `internal/dispatch/outcome.go` (`outcomeSummary`) | Why `{summary}` must not be forwarded to chat as is |
| Durable notice receipts: `queued`/`delivered`/`failed`/`stale`, lease, oldest-first per recipient, backoff 1 min doubling to 1 h, stable IDs | `internal/dispatch/orchestrator_notice.go` (`OrchestratorNotice`, `flushOrchestratorNotices`, `noticeBackoff`) | The chat route's queue |
| Receipts under `notices/KIND/HASH.json`; files directly in `notices/` are skipped by the loader | `internal/dispatch/orchestrator_notice.go` (`queueOrchestratorNotice`, `loadOrchestratorNotices`) | Where chat receipts and the thread file live |
| Reapply marks queued receipts `stale` | `internal/dispatch/orchestrator_notice.go` (`staleOrchestratorNotices`); `internal/dispatch/orchestrator.go` | Same for chat receipts |
| Pull route never executes a transport | `internal/dispatch/orchestrator_notice.go` (`flushOrchestratorNotices`, `PendingOrchestratorNotices`) | Amended: chat receipts are excluded from pull |
| The model's `reply.md` is queued as a notice | `internal/dispatch/orchestrator_notice.go` (`orchestratorReply`) | Never put on a chat route |
| A second store reusing the receipt type and writer, with retention (100 per sender, 14 days) | `internal/dispatch/member_notice.go` | Pattern for the node outbox for plain runs |
| Printable-only, whitespace-collapsed, length-capped text from untrusted notices | `internal/dispatch/member_notice.go` (`LocalMemberNotice`) | The cleaning step for agent-written fields |
| Spec fields `reply`, `notify_route`, `authority` (`notify` gates `reply.md`) | `internal/dispatch/orchestrator_spec.go` | New `notify:` mapping sits beside them |
| Lifetime start set on apply, unchanged by rotation | `internal/dispatch/orchestrator.go` (`st.Start` on apply), `internal/dispatch/repeat.go` | Thread anchor key |
| Inventory rows carry `outcome`, `key`, `question`, `reason`, `pr`, `validation` | `internal/dispatch/rpc.go` (`Row`) | Source of managed-run events |
| Members (RFC 0011 keys) cannot use orchestrators; orders have no notify field | `docs/nodes.md`; `internal/dispatch/order.go` (`Order`) | What a member may name |
| Secret scrubber (gitleaks rules plus Slack `xox` tokens, JWTs, Authorization values), HMAC-marked; `Load` returns nil on `[capture] redact = false` | `internal/redact/README.md`, `internal/redact/redact.go`, `internal/redact/config.go` | The redaction pass; notify scrubs even when capture opts out |
| `op://` references, memory-only cache (default TTL 5 min), batching, rate-limit cooldown, direct fallback | `internal/secrets/cache.go`, `internal/secrets/ipc.go`; RFC 0009 | Credential resolution |
| Target policy read from the target's own default config, never from a sender's `config_path` | `docs/dispatch.md` (admission); `internal/dispatch/admission.go` (`loadTarget("", "")`) | Same rule for `notify.toml` |

## Proposed surface

### 1. Named notifiers in node config

**Decision: change the sketch.** Keep named notifiers with a kind, a
credential reference and the channels each may post to. Put them in a new
file, `notify.toml`, not in `dispatch.toml`. Leave the `notify` argv in
`dispatch.toml` exactly as it is.

**Why not in `dispatch.toml`.** `config.go` decodes `notify` two ways today:

- `Load` (non-target) decodes the whole file into `Config` with
  `toml.DecodeFile(path, &c)`, where `Notify []string`. This is the path used
  at run completion (`Load(m.ConfigPath)` in `internal/dispatch/run.go`), by
  orchestrators (`internal/dispatch/orchestrator.go`) and by the sending CLI
  (`cmd/hev/dispatch.go`).
- `loadTarget` decodes `notify` as a `toml.Primitive` and skips undecoded
  `notify`, `validation` and `default_host` keys. Every other undecoded key is
  an error (`unknown config key`).

Checked against the current binary: a `[notify.team]` table fails `Load` with
`toml: (last key "notify"): incompatible types: TOML value has type
map[string]any; destination has type slice`, and a new `[notifiers.team]`
table fails with `unknown config key: notifiers.team`. Any in-file form
therefore breaks every older `hev` that reads that file: run completion
records `notify_error: target notification config: …`, and the sending CLI
refuses to start. A mixed-version fleet, or a rollback, would lose dispatch,
not just notify. The starting sketch's "the bare array keeps working as an unnamed command
notifier" is true of the separate-file design and false of the in-file one.

**Where the tables live.** `notify.toml` sits in the same directory as the
target's default dispatch config: the directory of `HEV_DISPATCH_CONFIG` if
set in the posting process, else `~/.config/hev/`. Like admission policy
(`docs/dispatch.md`), it is never selected by a sender's `--config` or a host's
`config_path`. (The run-end argv, by contrast, is read from `m.ConfigPath`,
which `config_path` can redirect. That stays as is.) A missing file means no
notifiers. A broken file breaks only notify: dispatch never reads it.

```toml
# ~/.config/hev/notify.toml (this node only)
default = "team"            # optional: used when a spec gives a channel but no notifier

[notifiers.team]
kind = "slack"
token = "op://example-vault/kit-slack-bot/credential"
audience = "internal"       # internal (default) | external
channels = { builds = "C0123456789", ops = "C0987654321" }

[notifiers.client-a]
kind = "slack"
token = "op://client-a-vault/slack-bot/credential"
audience = "external"
repos = ["https://github.com/ExampleClient/app"]   # optional: only runs in these repos
channels = { delivery = "C0AAAAAAAAA" }

[notifiers.alerts]
kind = "discord"
channels = { alerts = "op://example-vault/discord-alerts-webhook/url" }

[notifiers.relay]
kind = "command"
command = ["/usr/local/bin/post-to-chat", "--channel", "{channel}"]
token = "op://example-vault/relay/credential"   # optional; given to the command as $HEV_NOTIFY_TOKEN
channels = { general = "general" }
```

Rules:

- Names and labels follow `ValidLabel` (`internal/dispatch/order.go`).
- For `slack`, a channel value is a channel ID. Slack advises IDs over names
  ([chat.postMessage](https://docs.slack.dev/reference/methods/chat.postMessage)),
  and IDs survive renames, so the allowlist cannot drift onto a renamed
  channel.
- For `discord` and `teams`, a channel value is a secret reference to that
  channel's webhook URL. The URL is the credential, so there is no separate
  `token`.
- `command` keeps an escape hatch for other chat apps and for tests. The
  composed text goes to stdin. The first line of stdout, if any, is the
  message reference. The argv is executed without a shell, like `notify`
  today.
- Decoding is strict. Unknown keys are errors, reported by `hev notify check`
  and on the receipt, never printed with a credential value.

**How the two coexist.** `dispatch.toml`'s `notify` stays one argv run at run
end and on a verdict, with today's placeholders. It is, in effect, the node's
unnamed command notifier for plain runs. It does nothing for orchestrators.
To send a plain run to a named notifier, point that argv at the new verb
(section 7): `notify = ["hev", "notify", "post", "team", "--channel",
"builds", "--run", "{name}"]`. An existing host that changes nothing keeps
working byte for byte, because no file it reads has changed.

### 2. The node permits, the sender picks

**Decision: keep, with labels instead of raw channels and a mapping instead of
two loose keys.** An orchestrator spec gains one optional mapping:

```yaml
notify:
  notifier: team        # optional; the node's `default` when omitted
  channel: builds       # required to post: a label in that notifier's channel map
  events: [default]     # optional; see section 6
```

Spec decoding uses `KnownFields(true)` (`internal/dispatch/orchestrator_spec.go`),
so an older binary refuses a spec with `notify:` rather than silently dropping
the field.

- **Apply checks it.** `orchestrate apply` (and `--dry-run`) refuses an
  unknown notifier, an unknown label, or a notifier whose `repos` excludes a
  repo in `manages.repos`. This is the same fail-early rule `apply` already
  uses for an invalid notice sender (`noticeSenderAddress`).
- **Later breakage only fails the post.** If `notify.toml` changes after
  apply, the receipt is written `failed` with the reason (`notifier team not
  defined on this node`, `channel builds not allowed`) and the orchestrator
  carries on.
- **`notify:` does not need `reply` or `authority: notify`.** Chat posts are
  code-composed, like lifecycle notices, which already need neither
  (`docs/orchestrators.md`). The `notify` authority keeps its one meaning:
  the model's `reply.md` may go to `reply`. It never goes to chat.
- **Members name nothing.** RFC 0011 keys cannot apply orchestrators
  (`docs/nodes.md`), and orders have no notify field (`Order` in
  `internal/dispatch/order.go`). A member's run reaches chat only if the
  node owner's own `notify` argv posts it. The sketch's "specs can come from
  invited teammates" does not hold on `main`. If a later RFC lets members
  apply specs, a notifier would need an explicit `members = ["NAME"]` list,
  default empty.
- **Owner default.** `default` in `notify.toml` names the notifier used when a
  spec gives `channel` without `notifier`. It does **not** make orchestrators
  that never asked start posting: RFC 0008's "default off" stands. A spec
  with no `notify:` mapping posts nothing.

### 3. One delivery path

**Decision: keep.** A chat post is a receipt on the existing queue
(`internal/dispatch/orchestrator_notice.go`). `queueOrchestratorNotice`,
`flushOrchestratorNotices`, the lease, oldest-first ordering, backoff and
`stale`-on-reapply all apply unchanged. What changes:

- **`To`** is `notify:NOTIFIER/LABEL`, for example `notify:team/builds`. The
  `notify:` prefix cannot be a peers address: peers sender labels allow only
  letters, digits, `-`, `_` and `.` (`validNoticeSender`), and `apply` will
  refuse a `reply` that begins with `notify:`. The hash already covers
  `kind + key + to`, so one event sent to peers and to chat gives two receipts
  with independent status.
- **Kinds** get a `notify` directory: `notices/notify/HASH.json`. The ID
  stays `HOST/o-NAME:notify:HASH`.
- **Sender selection.** The flush's `send` dispatches on the `To` prefix:
  peers recipients use `sendOrchestratorNotice` as today; chat recipients use
  the notify sender for the notifier's kind. Both get the 20-second deadline.
- **Reply gate.** `queueOrchestratorNotice` returns early when `s.Reply == ""`.
  Chat receipts skip that check. The scope-only-target check stays: a
  scope-only registration never posts; the owner host speaks for the
  orchestrator.
- **Pull route.** `notify_route: pull` keeps its meaning for peers recipients
  only. Chat receipts are always pushed by the supervisor host, because
  posting needs only outbound HTTPS from that host. `PendingOrchestratorNotices`
  and `AckOrchestratorNotices` exclude `notify:` recipients, so a laptop pull
  can never pour chat receipts into a peers inbox, or acknowledge one unsent.

**Failure mapping.**

| Failure | Receipt | Why |
| --- | --- | --- |
| Network error, timeout, HTTP 5xx | stays `queued`; `attempts` +1; normal backoff | Transient |
| Rate limit: Slack HTTP 429 with `Retry-After` ([rate limits](https://docs.slack.dev/apis/web-api/rate-limits)); Discord 429 with `retry_after` ([rate limits](https://docs.discord.com/developers/topics/rate-limits)); Teams throttling over four requests a second ([incoming webhook](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook)) | stays `queued`; `next_attempt` = the later of backoff and the server's retry time; does **not** count toward expiry | The server says when |
| `hev secrets` cooldown | stays `queued`; `next_attempt` = the cooldown's retry-after (RFC 0009) | Never bypass a cooldown |
| Credential refused: Slack `invalid_auth`, `token_revoked`, `account_inactive`, `missing_scope`; Discord 401 or `50027` Invalid Webhook Token ([status codes](https://docs.discord.com/developers/topics/opcodes-and-status-codes)) | stays `queued` with backoff, and the next attempt asks `hev secrets` for a fresh read; becomes `failed` 24 h after `created` | The fix (rotating the secret) happens outside kit, and the receipt should go out once it is fixed. A bounded age stops it retrying forever |
| Channel refused: Slack `channel_not_found`, `not_in_channel`, `is_archived`; Discord 404 / `10015` Unknown Webhook ("If a webhook returns a 404, stop using it", [rate limits](https://docs.discord.com/developers/topics/rate-limits)) | `failed` | Retrying the same payload to the same place cannot succeed. Later receipts still try once each |
| Payload refused (Slack `no_text`, `msg_blocks_too_long`; Discord `50035`) | `failed` | A kit bug; the templates in section 6 stay well under every limit |
| Not configured: notifier or label missing, `repos` excludes the run, no credential source | `failed` when queued, with no request made | The node does not permit it |
| Reapply | `stale` | As today |

`failed` and `stale` stay terminal, as they are today. Expiry adds no new
state: an expired receipt is `failed` with `error: expired after 24h:
LAST_ERROR`. Error text is the platform's error code and HTTP status, cut
with `noticeText`. It is never a raw response body or a Go `url.Error`, whose
text includes the request URL. For a webhook, that URL is the secret.

**A post cannot change run state.** Orchestrators only queue under their
locks and deliver after releasing them. "Delivery errors never fail a round"
(`docs/orchestrators.md`; `flushOrchestratorNotices` discards the send error
after updating the receipt). Chat receipts use the same function, so the same
holds. For plain runs, `hev notify post` exits 0 once the receipt is durably
queued, whether or not the first attempt delivered. A non-zero exit means
only "not queued" (bad arguments, unknown notifier), which `run.go` records
as `notify_error`. `docs/dispatch.md` already guarantees that cannot change
the run state. Nothing in the notify path writes `result.json`, `state.json`
or `decisions.json`.

### 4. The interface

**Decision: keep, smaller.** Every kind implements one call:

```text
post(channel, text, thread?) -> (ref, threadRef, error)
```

`text` is plain text kit composed. `thread` is an opaque anchor from an
earlier post. `ref` identifies the message posted, when the platform returns
one. `threadRef` is the anchor later posts should use, which may be the
same. The error is classified as transient, rate-limited (with a retry time),
credential, channel or payload, as in section 3. There is no edit, delete,
reaction, upload, mention, rich layout or read in v1.

| Kind | Transport | Channel chosen by | Threads | Message ref | Mentions suppressed by | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| `slack` | Bot token, `chat.postMessage`, scope `chat:write` ([method](https://docs.slack.dev/reference/methods/chat.postMessage)) | channel ID from the label | Yes: `thread_ts` = the root's `ts` | `channel` + `ts` from the response | Escaping `&`, `<`, `>` ([formatting](https://docs.slack.dev/messaging/formatting-message-text)); no `link_names` | 1 message a second per channel ([rate limits](https://docs.slack.dev/apis/web-api/rate-limits)). kit does **not** ask for `chat:write.public` ([scope](https://docs.slack.dev/reference/scopes/chat.write.public)): the app must be invited, so Slack membership is a second gate behind the allowlist |
| `discord` | Channel webhook, Execute Webhook with `wait=true` ([webhook](https://docs.discord.com/developers/resources/webhook)) | the webhook | Degraded: in a forum channel the first post creates a thread (`thread_name`) and later posts use `thread_id`. In a text channel posts are flat, each prefixed with the subject | message `id` (with `wait=true`) | `allowed_mentions: {"parse": []}` ([message](https://docs.discord.com/developers/resources/message)) | 2,000-character `content` limit |
| `teams` | Workflows webhook ("When a Teams webhook request is received"); Office 365 Connectors were retired in May 2026 ([webhooks and connectors](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/what-are-webhooks-and-connectors)) | the webhook | No: posts are flat, each prefixed with the subject | None documented for the caller ([Teams connector](https://learn.microsoft.com/en-us/connectors/teams/)); receipt `ref` stays empty | kit sends no mention entities | 28 KB payload limit ([incoming webhook](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook)). Graph can post replies only with delegated permission, and application permission is for migration only ([channel-post-messages](https://learn.microsoft.com/en-us/graph/api/channel-post-messages)), so an unattended threaded Teams kind would need a Bot Framework bot: a later kind |
| `command` | argv, no shell, text on stdin | `{channel}` placeholder | If the command prints a reference and accepts `{thread}` | first stdout line | The command's job | For other apps and for tests |

Left out on purpose: interactive elements (they are inbound), message
updates (Alertmanager's `update_message` style), per-user mentions (a
decision does not page anyone in v1), file attachments, and any kind that
needs a listener.

### 5. Threading

**Decision: keep, with the anchor keyed to the orchestrator lifetime.**

- **Orchestrators.** The first chat receipt of an orchestrator lifetime
  (normally `started`) posts at top level. Its `threadRef` is written to
  `~/dispatch/orch-NAME/notices/threads.json`, keyed by
  `NOTIFIER/LABEL` and the lifetime start (`st.Start`). Every later receipt
  for that key posts as a reply. The file sits directly in `notices/`, which
  `loadOrchestratorNotices` already skips, as it does `cap-episode.json`. It
  is written under the notices lock with the same fsync-and-rename as
  receipts (`writeNotice`).
- **Supervisor restart.** The anchor is on disk, so a restarted supervisor
  keeps replying in the same thread.
- **Rotation.** `rotate_after` starts a new lineage but does not change
  `st.Start` (`internal/dispatch/orchestrator.go` sets it only on apply), so
  rotation stays in the same thread.
- **Handoff.** `handoff --to` posts its event in the thread and ends the
  supervisor. The anchor stays for anyone reading back.
- **Reapply.** A reapply starts a new lifetime (`st.Start` changes, queued
  receipts go `stale`), so it gets a new thread. The new root links nothing
  from the old one, so an old thread cannot be posted into by a new spec
  whose audience may differ.
- **Ordering.** Replies need the root's reference. The flush already goes
  oldest first per recipient and stops at the first failure, so a reply is
  never attempted before its root has been delivered.
- **Lost anchor.** If `threads.json` is missing or unreadable, or the
  platform refuses the anchor, kit posts the receipt at top level with the
  prefix `(new thread)` and records the new anchor. It never guesses an
  anchor from channel history (that would be a read). For kinds without
  threads there is no anchor, and every post carries its subject.
- **Plain runs.** One thread per run, keyed by run name and run start,
  stored at `~/dispatch/NAME/notify-thread.json`. Rework reuses the run
  directory, so a decision, its rework and the final result share one thread.

### 6. What gets posted, and its exact shape

**Decision: fixed templates per event, chosen by event name; no user
templates.** Templates are code. Each field is either kit-owned (an enum, a
number, a time, a validated name, a URL kit read from `gh`) or agent-written
and handled by the rules below.

**Events.** "Default" is what `events: [default]` (or no `events`) selects. A
spec can list events by name, or `all`.

| Event | Source (state only) | Default | Template (internal) | External notifier |
| --- | --- | --- | --- | --- |
| `started` | apply/start lifecycle; `st`, spec | on | `Orchestrator {name} started on {host}. Goal: {goal}. {mode} every {every} until {until}. Details: hev orchestrate show {name}` | no `on {host}` |
| `round` | accepted round; `st.Rounds`, `managed_runs`, counts from inventory | off | `Round {n} of {name}: {managed} runs ({live} live, {decisions} awaiting a decision, {blocked} blocked). Next wake {next_wake}.` | same |
| `needs-decision` | a managed run's row newly has `outcome=needs-decision` (key: run, key, run start) | on | `{run} needs a decision ({key}): {question}. Answer from a shell: hev dispatch say {run} --resolve {key} 'ANSWER'` | `{run} needs a decision ({key}). The question is withheld from this channel; see hev dispatch show {run}.` |
| `resolved` | a decision in `decisions.json` newly has `resolved` | on | `{run}: decision {key} answered by {resolved_by}; rework started.` | no `by {resolved_by}` |
| `blocked` | row newly `blocked` | on | `{run} is blocked: {reason}. See hev dispatch show {run}.` | `{run} is blocked. The reason is withheld from this channel; see hev dispatch show {run}.` |
| `done` | row newly `done` | on | `{run} done.{ PR: {pr}}` | same |
| `failed` | row newly `failed` or `timeout` | on | `{run} {state} (exit {exit}).` | same |
| `verdict` | row's `validation` newly final | on | `{run} validation {verdict}.{ PR: {pr}}` | same |
| `round-failures` | second consecutive round failure or timeout | on | `{name}: {n} consecutive rounds failed or timed out. See hev orchestrate show {name}.` | same |
| `budget` | budget exhaustion lifecycle | on | `{name} stopped admitting runs: {cap} reached.` | same |
| `handoff` | `handoff --to` | on | `{name} handed off to {to}; this supervisor has stopped.` | `{name} handed off; this supervisor has stopped.` |
| `complete` | goal met | on | `{name} met its goal after {rounds} rounds.` | same |
| `stopped` | stop | on | `{name} stopped.` | same |
| `paused` / `resumed` | controls | off | `{name} paused.` / `{name} resumed.` | same |
| `source-cap`, `intake` | source cap episode, intake health signals | off | `{name}: intake {signal} for event {event}.` (no plugin error text) | not available |

Plain runs (section 7) use `needs-decision`, `blocked`, `done`, `failed` and
`verdict`, with the same templates, and are always on, because the owner
chose to call `post --run`.

**Field rules.**

| Field | Written by | Rule |
| --- | --- | --- |
| `{name}`, `{run}` | operator or orchestrator; charset already enforced by `ValidName` | Truncate to 64 runes |
| `{host}` | operator config label | Internal only (host labels name operator machines) |
| `{state}`, `{mode}`, `{cap}`, `{signal}` | code enums | As is |
| `{verdict}` | validator, but only `pass` or `fail` is accepted (`internal/dispatch/validation.go`); item notes are never posted | As is |
| `{key}` | worker, but restricted by `ValidDecisionKey` (40 characters, safe charset; `internal/dispatch/outcome.go`) | As is |
| `{pr}` | kit, from `gh pr view` | Must parse as an `https` URL, else dropped |
| `{goal}` | the person who applied the spec | Cleaned and scrubbed; 200 runes |
| `{to}`, `{resolved_by}` | peers address; authenticated sender | Internal only |
| `{question}` (decision) | **agent** | Cleaned, scrubbed, escaped; 280 runes then `…`; **withheld** on external |
| `{reason}` (block) | **agent** | As `{question}` |
| Status marker word | agent, but parsed into the enum `outcome` | Only the enum is used. Text after `done:` is never used |
| Intake and round error text | plugin, harness or code | Never posted; pointer to `show` instead |
| `reply.md`, checkpoint, report, log, PR title/body | agent | Never posted |

"Cleaned" is the `LocalMemberNotice` treatment (`internal/dispatch/member_notice.go`):
printable runes only, whitespace collapsed. "Scrubbed" is a pass of
`internal/redact`'s `Scrubber.Text`. Notify builds its scrubber with the
installation salt even when `[capture] redact = false`, because
`redact.Load` returns a nil scrubber in that case (`internal/redact/config.go`).
Replacement counts are recorded on the receipt. Redaction catches secrets,
not confidential prose. That is why agent fields are withheld from external
notifiers rather than merely scrubbed. "Escaped" means the kind's
neutralisation from section 4, so a question containing `<!channel>` or
`@everyone` cannot ping anyone. The whole message is capped at 1,000 runes
(the same bound as `memberNoticeText`), below Slack's 4,000-character
guidance and Discord's 2,000-character limit.

The composed text is stored as the receipt's `message` before the first
attempt. As today, payload and recipient are then fixed across retries.

### 7. Plain runs: `hev notify post` and `hev notify check`

**Decision: keep, with a run mode so the argv never carries agent text.**

```sh
hev notify post NOTIFIER --channel LABEL --run NAME     # compose from ~/dispatch/NAME
hev notify post NOTIFIER --channel LABEL --test         # fixed test message
hev notify post NOTIFIER --channel LABEL --text 'TEXT'  # internal notifiers only
hev notify check [NOTIFIER] [--json]                    # no post
hev notify ls [--json]                                  # notifiers and labels, no values
hev notify flush                                        # retry queued receipts on this node
```

- `--run NAME` reads `meta.json`, `result.json` and `decisions.json`, picks
  the event from state (needs-decision, blocked, done, failed/timeout, or
  verdict when called from the validation notify), and uses section 6's
  templates. This is the form for `dispatch.toml`:
  `notify = ["hev", "notify", "post", "team", "--channel", "builds", "--run", "{name}"]`.
  Forwarding `{summary}` instead would forward the agent's marker text
  verbatim, which is what decided point 4 rules out.
- Because `notify` is one argv, a host that wants both a peers notice and a
  chat post points it at a small script that runs both. A `[runs]` table in
  `notify.toml` that kit calls directly is an open question.
- `--test` posts `kit test from notifier NOTIFIER at TIME.` It is the only
  way to post to an external notifier by hand.
- `--text` is refused on `audience = "external"`. On internal notifiers it is
  cleaned, scrubbed and capped like an agent field: whoever ran the command
  approved it.
- Receipts for plain runs go to a node outbox, `~/dispatch/.notify/outbox/HASH.json`,
  with the same type, writer, lease and backoff, and member-notice-style
  retention (`internal/dispatch/member_notice.go`). The ID is
  `HOST/d-NAME:notify:HASH`, keyed by run, run start, event and recipient, so
  the run-end call and the validation call can never post the same event twice.
- **Retry without a daemon.** `post` makes one attempt, then returns. Queued
  receipts are retried by the next `hev notify post` or `hev notify flush` on
  the node, and by any orchestrator supervisor there when it flushes. A node
  that runs nothing after a failure keeps the receipt queued until someone
  runs `flush`. Wiring `flush` into `hev daemon` is an open question.
- `check` does not post. It decodes the file strictly and resolves each
  credential through `hev secrets`, printing only "resolved" or the
  classified error. For Slack it calls `auth.test` (no scopes needed;
  returns team and bot identity, [auth.test](https://docs.slack.dev/reference/methods/auth.test)),
  then `conversations.info` for each label, reading `is_member`
  ([conversation object](https://docs.slack.dev/reference/objects/conversation-object)).
  That needs `channels:read`/`groups:read`; without them `check` reports
  "membership unknown" instead of failing. Discord and Teams webhooks offer
  no read-only probe kit should rely on, so `check` reports "webhook
  resolved; delivery unverified; use post --test". `check` never prints a
  token, webhook URL or channel ID, only labels and workspace or team name.

### 8. Secrets

**Decision: keep.** A notifier's `token` (or a webhook channel value) is a
reference. kit never accepts a literal value: `xox…` tokens and `https://`
webhook URLs in `notify.toml` are refused by the decoder.

- **Accepted forms.** `op://…`, resolved through `secrets.Resolve`
  (`internal/secrets/ipc.go`), which validates `op://` references
  (`internal/secrets/cache.go`). Also `env:NAME`, read from the posting
  process's environment, for nodes without a secrets provider. If the
  open `secret://NAME` docs proposal (PR #219) lands, it is accepted too.
- **Who resolves.** The host that posts, and only at send time: the
  orchestrator's supervisor host, or the run's host for `hev notify post`.
  No reference or value crosses SSH or an RPC.
- **Where values go.** For `slack`, `discord` and `teams`, the value stays in
  the posting process's memory and goes into the request header or URL. For
  `command`, it goes only in that child's `HEV_NOTIFY_TOKEN` environment,
  never in argv. RFC 0009's rules apply as written: no disk, logs, arguments
  or archives.
- **`env:` names are stripped from other children.** They join the strip list
  that `source.env` names already use (`docs/orchestrators.md`), so worker
  harnesses and helpers do not inherit them.
- **Cache and rate limits.** A flush resolves every reference it needs in one
  `secrets.Resolve` batch (RFC 0009 batching). It relies on the endpoint's
  default 5-minute TTL, so a burst of retries costs one `op` read. Only a
  credential-class failure triggers a fresh read, at most once an hour per
  notifier. A secrets cooldown keeps the receipt queued until the cooldown's
  retry time (section 3).
- **No provider.** With `op://` and no service account, or no running
  endpoint, direct mode runs once per invocation (RFC 0009). If that also
  fails, the receipt is `failed` with `no secrets provider for notifier
  NAME`, and `check` says the same. Nothing falls back to a desktop app.
- **Never in errors.** Errors name the notifier and the error class, never
  the reference URI. RFC 0009 already keeps reference URIs out of logs.

### 9. Scope and trust

**Decision: keep; the boundary is the file plus the absence of any remote
path.**

- **Only local config is consulted.** A notifier name is resolved only
  against the posting process's own `notify.toml`. No RPC action carries a
  notifier, a channel value or a credential, and v1 adds none. Orders have
  no notify field. So a run or orchestrator on one node cannot post through
  another node's notifier.
- **Where an orchestrator posts.** It posts through its supervisor host's
  notifiers, and that host may differ from where managed runs execute. To keep
  a client's run state in the client's chat and nowhere else, place the
  orchestrator on the client's node (`host:`) and give the notifier a `repos`
  list. Events for runs outside `repos` are dropped as `failed: repo not
  allowed`.
- **Client nodes.** A node used for client work carries only that client's
  notifier, which is an operator practice. kit helps with `audience =
  "external"` (agent text withheld), `repos`, and Slack membership as a
  second gate (no `chat:write.public`).
- **Not a sandbox.** RFC 0011's boundary applies unchanged. A worker agent
  runs as the node's account and can read `notify.toml`, and can call
  `hev notify post` or `op` itself. kit guarantees what *kit* posts, not what
  a hostile agent on the same account can do. Isolating that needs a separate
  OS account or machine, as RFC 0011 says.

**Audit.** Each receipt records:

- `notifier`, `channel` (label), `kind`, `event`
- `requested_by`: the run's `requested_by` from `meta.json`; the account that
  applied the spec, recorded at apply as `local:ACCOUNT`; or the shell user
  for `post --test`/`--text`
- `message` as posted
- `redactions` (counts) and `withheld` (field names dropped for an external
  audience)
- `ref` and `thread` (message references, which are not secrets)
- status, attempts and error

Delivered and failed receipts also append one line (ID, time, notifier,
label, event, status, ref, message SHA-256) to `~/dispatch/.notify/audit.jsonl`.
That outlives receipt pruning without keeping message text twice.

### Statements this RFC amends

- **RFC 0008, "Owner communication contract":** "All require `reply`" becomes
  "all peers notices require `reply`; chat notices require a `notify:`
  mapping instead". "`notify_route: pull` never executes a transport" becomes
  "never executes a peers transport; chat receipts are pushed by the
  supervisor host". The receipt `KIND` list gains `notify`.
- **RFC 0008, "Stable orchestrator inventory":** `last_notice` may be a chat
  receipt. `to` may begin with `notify:`, and the receipt gains the audit
  fields above. Consumers already must ignore added fields.
- **RFC 0008 spec:** adds the optional `notify:` mapping, so
  `schemas/orchestrator.schema.json` needs the same change.
- **`docs/orchestrators.md`, "Owner notices":** the `KIND` list, the reply
  gate and the pull-route sentence, as above. The `source.env` strip list
  also covers notifier `env:` names.
- **`docs/dispatch.md`:** documents `notify.toml` beside admission policy, and
  `hev notify post --run` as the recommended argv for chat.
- **RFC 0009:** a second consumer of `secrets.Resolve`, in-process rather than
  `secrets run`. The trust rules are unchanged, and `env:` is added as a
  documented no-provider form.
- **RFC 0011:** members cannot name notifiers. A member run reaches chat only
  through the owner's `notify` argv, which `queueMemberNotice` already runs
  for templates without `{reply}`.

## End-to-end example

**Node config** (`~/.config/hev/notify.toml` on `build-1`):

```toml
[notifiers.team]
kind = "slack"
token = "op://example-vault/kit-slack-bot/credential"
channels = { builds = "C0123456789" }
```

`hev notify check team` prints `team: slack, auth ok (workspace Example), builds: member`.

**Spec** (`release.md`):

```yaml
---
kind: orchestrator
name: release-train
goal: Every PR labelled release-0.9 is merged with green CI.
host: build-1
mode: loop
every: 20m
until: goal
max_lifetime: 7d
harness: claude
reply: my-session
notify:
  notifier: team
  channel: builds
manages:
  runs: {selector: "issue=REL-*"}
authority: [say, notify, rework, retry]
---
Keep the release moving.
```

1. `hev orchestrate apply -f release.md` checks `team` and `builds`, then
   queues `lifecycle` (peers, to `my-session`) and `notify` (`To:
   notify:team/builds`). The supervisor posts the root:
   `Orchestrator release-train started on build-1. Goal: Every PR labelled
   release-0.9 is merged with green CI. loop every 20m until goal. Details:
   hev orchestrate show release-train`. The receipt is `delivered`, with
   `ref: C0123456789/1700000000.000100`, and `threads.json` stores that as the
   anchor.
2. Round 4 gathers run `rel-42` with `outcome=needs-decision`, `key=db`,
   `question=Postgres or SQLite for the cache?`. Code queues
   `needs-decision`, and it posts in the thread:
   `rel-42 needs a decision (db): Postgres or SQLite for the cache? Answer
   from a shell: hev dispatch say rel-42 --resolve db 'ANSWER'`.
3. The owner answers from a shell: `hev dispatch say rel-42 --resolve db
   'SQLite'`. The next round sees the resolved decision and posts in the
   thread: `rel-42: decision db answered by local:owner; rework started.`
   Nobody can answer from Slack, and the post says so by pointing at the shell.
4. The rework finishes with a PR. The thread gets `rel-42 done. PR:
   https://github.com/example/app/pull/42`, then `rel-42 validation pass.`
   The goal is met: `release-train met its goal after 9 rounds.`

**Same run with the token revoked** before step 2:

- The `needs-decision` post gets `token_revoked`. The receipt stays
  `queued`, with `attempts: 1` and `error: slack: token_revoked`. The next
  attempt in about 1 minute asks for a fresh secret read; backoff continues.
- `hev orchestrate ls --json` shows it in `last_notice`. The peers notice to
  `my-session` is a separate receipt and is unaffected.
- The round, the run and the decision are untouched: `rel-42` stays
  `needs-review`, and the owner can still resolve from the shell.
- The owner rotates the token in the vault. The next attempt after the
  5-minute cache expiry, or after the fresh read, delivers the queued
  receipts in order: needs-decision first, then any later events.
- If nobody rotates it, each receipt becomes `failed` 24 hours after it was
  created, with `error: expired after 24h: slack: token_revoked`.
  `hev notify check team` prints `team: auth failed (token_revoked)`.

## Borrow and refuse: CI systems and Alertmanager

| From | Borrow | Refuse |
| --- | --- | --- |
| Alertmanager receivers ([configuration](https://prometheus.io/docs/alerting/latest/configuration/)) | Named receivers referenced by name from routes. Secrets from a file or reference (`api_url_file`, `app_token_file`), not inline. `send_resolved` becomes the `resolved` event. Threading follow-ups (`post_updates_to_thread`) | User-written Go templates (`title`, `text`): kit's text must come only from state. A central routing tree: config is per node. Grouping timers (`group_wait`, `group_interval`): kit events are few and keyed, and stable keys already dedupe |
| GitLab for Slack ([docs](https://docs.gitlab.com/user/project/integrations/gitlab_slack_application/)) | Per-event toggles with a destination per event. A private channel requires the app to be added first: the same membership gate kit relies on | Configuring the destination in the thing being built (the project). In kit, the node decides; an order cannot choose |
| Buildkite `notify` ([docs](https://buildkite.com/docs/pipelines/configure/notifications)) | The pipeline picks a channel, but only among what an admin-configured integration can reach: the "node permits, sender picks" split | Free-form channel names in pipeline YAML. kit uses node-defined labels |

## Build order

Each phase is one dispatchable order with its own acceptance.

1. **Slack outbound on one node.** Add the `notify.toml` decoder (strict,
   references only), the `slack` kind, the `notify:` spec mapping with apply
   checks, the chat route on the orchestrator queue (prefix dispatch, reply
   gate, pull exclusion, failure mapping, expiry), `threads.json`, the
   templates and field rules for orchestrator events, always-on scrubbing,
   audit fields, and `hev notify check` for Slack. Tests use a fake Slack
   server only, and docs are updated with the amendments. Acceptance: the
   end-to-end example, including the revoked token, passes against the fake.
2. **Plain runs.** `hev notify post --run/--test/--text`, the node outbox,
   `hev notify flush`, `hev notify ls`, and the per-run thread.
3. **Discord and Teams webhooks.** Both kinds, their degradations, and `check`
   output.
4. **`command` kind and `env:` credentials**, including the child-environment
   strip list.
5. **Optional:** a `[runs]` table that posts plain runs without an argv, and
   `flush` from `hev daemon`, if the open questions say yes.

## Later: inbound

Not built in v1. v1 must not block it.

- **Transport.** Slack Socket Mode needs an app-level `xapp-` token and no
  public URL ([Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode)).
  The Events API needs a public endpoint that answers within 3 seconds, with
  request signing ([Events API](https://docs.slack.dev/apis/events-api/),
  [verifying requests](https://docs.slack.dev/authentication/verifying-requests-from-slack)).
  Discord needs a Gateway WebSocket and a bot token
  ([Gateway](https://docs.discord.com/developers/events/gateway)). Teams needs
  a Bot Framework bot. Every option is a long-lived listener, which kit
  deliberately does not run today. That makes it a separate, opt-in process.
- **Who may say what.** A chat identity has to map to a kit identity before it
  can act: a per-notifier map from chat user ID to `local:ACCOUNT` or a member
  name. Each mapped identity gets a verb allowlist no wider than its shell
  rights. Resolve is the obvious first verb, routed into the same
  `hev dispatch say --resolve` admission, so the decision ledger records
  `resolved_by` as the mapped identity. No verb would be available to
  unmapped users, and nothing on an external notifier by default.
- **What v1 leaves in place for it.** Thread anchors are stored, so a reply in
  a thread can be tied to an orchestrator or run. `needs-decision` posts carry
  the run and key in a fixed shape. Receipts record `ref`, so an inbound
  handler can find what a message was about without parsing prose.

## Open questions

Each is for the decision-maker; the recommended default is in bold.

1. **External notifiers and agent text.** Withhold the decision question and
   block reason entirely, or post them scrubbed and capped? **Withhold**
   (decided point 4). An owner who wants them in a client channel can mark
   that notifier `internal`, a visible, deliberate choice.
2. **Default notifier scope.** Should `default` make every orchestrator on the
   node post without a `notify:` mapping? **No**, keep RFC 0008's default off.
   `default` only fills in a missing `notifier`.
3. **Plain runs without an argv.** Add a `[runs]` table to `notify.toml` so
   runs post without touching `dispatch.toml`'s single argv? **Not in v1**;
   use `post --run` from the argv, and revisit after phase 2.
4. **Member runs.** Should a notifier post member runs when the owner's argv
   calls `post --run`? **Yes**, because the owner wrote the argv. The message
   carries no member name on external notifiers.
5. **Credential-failure expiry.** 24 hours, then `failed`? **Yes.**
6. **Retries on idle nodes.** Have `hev daemon` call `hev notify flush`?
   **Yes, in phase 5**, only on nodes with a `notify.toml`.
7. **`repos` on external notifiers.** Require it? **Recommend but do not
   require.** `check` warns when an external notifier has no `repos`.
8. **New thread on reapply.** **Yes**, as designed. The alternative, one
   thread forever per orchestrator, risks very long threads and mixing a
   respec'd audience into old history.
