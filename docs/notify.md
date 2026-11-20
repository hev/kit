# Notify: post orchestrator events to Slack

A node can declare **notifiers**: named chat destinations with a credential
reference and the channels each may post to. An orchestrator spec picks one
notifier and one channel label with a `notify:` mapping. kit composes every
message itself from run and orchestrator state, scrubs it, and posts it
through the same durable notice queue as owner notices. Posting is outbound
only, and a failed post never changes a run or a round.

This is phase 1 of [RFC 0017](rfcs/0017-notify.md): Slack, on one node, for
orchestrators. Plain runs (`hev notify post`), Discord, Teams, the `command`
kind and `env:` credentials are not built yet.

## notify.toml

Notifiers live in `notify.toml` beside the node's own default dispatch
config: the directory of `HEV_DISPATCH_CONFIG` when it is set in the posting
process, else `~/.config/hev/notify.toml`. Like
[repository admission](dispatch.md#repository-admission), it is never chosen
by a sender's `--config` or a host's `config_path`. Dispatch never reads it,
so a missing or broken file affects only notify. A missing file means the
node has no notifiers.

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
repos = ["https://github.com/ExampleClient/app"]   # optional; check warns when an external notifier has none
channels = { delivery = "C0AAAAAAAAA" }
```

Decoding is strict:

- Unknown keys are errors.
- Notifier names and channel labels use lowercase letters, digits and `-`.
- `kind` is `slack`. Other kinds named by the RFC are refused by this version.
- `token` must be an `op://VAULT/ITEM/FIELD` reference. A literal token (for
  example one starting `xoxb-`) or a URL is refused, and the error never
  repeats the value.
- A Slack channel value is a channel ID (`C…`, `G…` or `D…`), never a name,
  so the allowlist cannot drift onto a renamed channel.

`dispatch.toml`'s `notify` argv is unchanged. It still runs once at run end
and on a verdict, and it does nothing for orchestrators.

## The spec mapping

```yaml
reply: my-session        # peers owner notices, unchanged
notify:
  notifier: team         # optional; notify.toml's default when omitted
  channel: builds        # a label in that notifier's channels
  events: [default]      # optional: default, all, or event names
```

`hev orchestrate apply` (and `--dry-run`) refuses an unknown notifier, a label
the notifier does not list, a notifier whose `repos` does not include a repo
in `manages.repos`, and a node with no `notify.toml`. Older binaries refuse a
spec with `notify:` rather than ignoring it.

`notify:` needs neither `reply` nor `notify` authority: chat posts are
composed by code. `notify` authority keeps its one meaning, that the model's
`reply.md` may go to `reply`; it is never posted to chat. `default` in
`notify.toml` only fills in a missing `notifier`; an orchestrator without a
`notify:` mapping posts nothing.

## Events and messages

Every message is a fixed template over state kit owns. The question of a
decision and the reason a run is blocked are written by an agent, so they are
cleaned (printable characters, whitespace collapsed), scrubbed with kit's
secret redaction, and capped at 280 characters. On a notifier with
`audience = "external"` they are withheld entirely, as is the host label.
`reply.md`, checkpoints, reports, logs, status text and PR titles and bodies
are never posted. A whole message is at most 1,000 characters, and `&`, `<`
and `>` are escaped so no text can form a Slack mention.

Scrubbing runs even when `[capture] redact = false`.

| Event | Default | Message (internal notifier) |
| --- | --- | --- |
| `started` | on | `Orchestrator NAME started on HOST. Goal: GOAL. MODE every EVERY until UNTIL. Details: hev orchestrate show NAME` |
| `round` | off | `Round N of NAME: M runs (L live, D awaiting a decision, B blocked). Next wake TIME.` |
| `needs-decision` | on | `RUN needs a decision (KEY): QUESTION Answer from a shell: hev dispatch say RUN --resolve KEY 'ANSWER'` |
| `resolved` | on | `RUN: decision KEY answered by WHO; rework started.` |
| `blocked` | on | `RUN is blocked: REASON See hev dispatch show RUN.` |
| `done` | on | `RUN done. PR: URL` |
| `failed` | on | `RUN failed (exit N).` or `RUN timeout (exit N).` |
| `verdict` | on | `RUN validation pass.` (or `fail`), with ` PR: URL` when the run has one |
| `round-failures` | on | `NAME: N consecutive rounds failed or timed out. See hev orchestrate show NAME.` |
| `budget` | on | `NAME stopped admitting runs: CAP reached.` |
| `handoff` | on | `NAME handed off to SESSION; this supervisor has stopped.` |
| `complete` | on | `NAME met its goal after N rounds.` |
| `stopped` | on | `NAME stopped.` |
| `paused`, `resumed` | off | `NAME paused.` / `NAME resumed.` |
| `source-cap`, `intake` | off | `NAME: intake SIGNAL for event EVENT.` (internal notifiers only) |

On an external notifier, `needs-decision` becomes `RUN needs a decision
(KEY). The question is withheld from this channel; see hev dispatch show RUN.`
and `blocked` becomes `RUN is blocked. The reason is withheld from this
channel; see hev dispatch show RUN.` Nobody can answer from Slack; the post
points at the shell.

Run events are keyed by run, run start and event, so each state is posted
once per run attempt by whichever round first sees it. A run that is already
done when an orchestrator first gathers it is posted once.

## Delivery

A chat post is a receipt in `notices/notify/HASH.json` with `to` set to
`notify:NOTIFIER/LABEL` and the ID `HOST/o-NAME:notify:HASH`. Queueing,
leases, oldest-first delivery per recipient and the 20-second send deadline
are the same as for owner notices. The supervisor host always posts chat
receipts: `notify_route: pull` applies to peers notices only, and the pull
route never lists or acknowledges a chat receipt.

| Failure | Receipt |
| --- | --- |
| Network error, timeout, HTTP 5xx | stays `queued`, backs off (1 minute doubling to 1 hour) |
| Slack HTTP 429 with `Retry-After` | stays `queued` until the later of the backoff and the server's retry time |
| `hev secrets` cooldown | stays `queued` until the cooldown's retry time |
| Credential refused (`invalid_auth`, `token_revoked`, `account_inactive`, `missing_scope`, …) | stays `queued`; the next attempt asks `hev secrets` for a fresh read (at most once an hour per notifier); `failed` with `expired after 24h: …` 24 hours after it was created |
| Channel refused (`channel_not_found`, `not_in_channel`, `is_archived`) | `failed` |
| Payload refused (`no_text`, `msg_blocks_too_long`) | `failed` |
| Not configured (notifier or label removed after apply, run outside `repos`, no secrets provider) | `failed`, with no request made |
| Reapply | `stale` |

Errors carry the Slack error code and HTTP status, never a response body, a
request URL, a reference or a token. Delivery errors never fail a round, and
nothing in the notify path writes `result.json`, `state.json` or
`decisions.json`.

Each chat receipt also records `notifier`, `channel` (the label),
`notifier_kind`, `event`, `requested_by` (the account that applied the spec),
`redactions` (`RULE=COUNT,…`), `withheld` (field names dropped for an external
audience), `ref` (`CHANNEL/TS` of the post), `thread`, `error_class` and
`lifetime`. Delivered and failed chat receipts append one line (ID, time,
notifier, label, event, status, ref and the message's SHA-256, not its text)
to `~/dispatch/.notify/audit.jsonl`.

## Threads

The first post of an orchestrator lifetime (normally `started`) is a
top-level message, and every later post replies in its thread. The anchor is
stored in `~/dispatch/orch-NAME/notices/threads.json`, keyed by
`NOTIFIER/LABEL` and the lifetime start, so a restarted supervisor, a
`rotate_after` rotation and a handoff stay in the same thread. A reapply
starts a new lifetime and a new thread. If `threads.json` is lost or Slack
refuses the anchor, kit posts at top level with the prefix `(new thread)` and
records the new anchor; it never reads channel history to find the old one.

## Credentials

`token` is resolved through [`hev secrets`](rfcs/0009-secrets.md) on the
posting host at send time, with the endpoint's 5-minute cache. The value
stays in memory and goes only into the request's `Authorization` header. It
is never written to disk, a receipt, an error or a log. With no service
account and no endpoint, the receipt fails with `no secrets provider for
notifier NAME`.

kit does not ask for `chat:write.public`: the Slack app must be invited to
each channel, so Slack membership is a second gate behind the channel list.
A worker agent running as the same account can still read `notify.toml` and
call `op` itself; notify guarantees what kit posts, not what a hostile agent
on the same account can do.

## Check a notifier

```sh
hev notify check [NOTIFIER] [--json]
```

`check` does not post. It decodes `notify.toml` strictly, resolves each
credential, calls Slack's `auth.test`, then `conversations.info` for each
label. It prints labels, the workspace name and classified errors, never a
token, reference or channel ID, and exits 1 when a notifier cannot post:

```text
team: slack, auth ok (workspace Example), builds: member
team: auth failed (token_revoked)
team: slack, auth ok (workspace Example), builds: not a member (invite the app to the channel)
team: slack, auth ok (workspace Example), builds: membership unknown (add channels:read and groups:read to check)
```

Without `channels:read` and `groups:read` the app cannot read membership;
`check` reports "membership unknown" instead of failing.

## Set up Slack by hand

1. Create a Slack app for the workspace, add the bot scope `chat:write`
   (optionally `channels:read` and `groups:read` so `check` can read
   membership) and install it. Copy the bot token.
2. Store the token in your secrets vault and note its `op://` reference.
3. Invite the app to each channel it should post to (`/invite @APP`), and
   copy each channel's ID from the channel details.
4. Write `notify.toml` as above, with the reference and channel IDs.
5. Run `hev notify check`, then add a `notify:` mapping to a spec and apply it.
