# "Needs you" notices

Kit raises one typed **attention notice** for every moment a person has to
look at something, on the host where it happens. Each notice:

- runs your `hev-hook-NAME` handlers as a run-lifecycle event
  ([hooks](hooks.md)), and
- posts to Slack when that host has `[notify.slack]`.

The existing peers delivery (dispatch `notify`, orchestrator owner notices,
`notify_route: pull`) is unchanged. Notices are added alongside it.

## When a notice is raised

| Kind | Source | Moment |
| --- | --- | --- |
| `needs-you` | `run` | a dispatch run ends `needs-decision`, `blocked`, `failed`, `timeout` or `needs-review` |
| `needs-you` | `steer` | a steering halt escalates to a hard stop ([steering](steering.md)) |
| `needs-you` | `orchestrator`, `expedite`, `swarm` | a round's `reply.md` starts `[needs you]` on its first line, or an expediter stalls |
| `needs-you` | `expedite` | a legacy `hev expedite round` line contains `[needs you]` |
| `ready` | `run` | a validation PASS lands on a run whose PR is ready (not draft) with all checks green |
| `ready` | `orchestrator`, … | an orchestrator meets its goal (`goal-met.json`) |
| `done` | `run` | a dispatch run ends `done` |
| `done` | `orchestrator`, … | an orchestrator is stopped |

A stop someone asked for and a halt that ended its turn in time raise
nothing. Validator children and repeating (`every`) runs raise nothing.
The orchestrator reply notice is raised whether or not the orchestrator has
`notify` authority. Authority still gates the peers owner notice.

## The notice

`hev.attention.event/v1` ([schemas/attention_event.schema.json](../schemas/attention_event.schema.json)):

```json
{
  "schema": "hev.attention.event/v1",
  "id": "att_5f0c9e1b2a7d4c3e8f6a1b2c",
  "time": "2026-11-17T03:12:09Z",
  "event": "needs-you",
  "source": "run",
  "name": "kit-needs-you-slack-20261117",
  "host": "oven",
  "state": "needs-review",
  "text": "needs-decision db: Postgres or SQLite?",
  "links": {"pr": "https://github.com/hev/kit/pull/260", "run": "hev dispatch show kit-needs-you-slack-20261117 --host oven"},
  "thread": "run:oven/kit-needs-you-slack-20261117"
}
```

- `id` is derived from what the notice is about: source, host, name, kind, and
  the moment (for a run, its attempt start and end). It never comes from the
  emit time. Emitting the same moment again does nothing: not after a retry,
  not after a supervisor restart, and not when a pulled receipt is redelivered.
- `text` is one line, with whitespace collapsed, credentials redacted and at
  most 240 characters. It never holds board bodies or repository content.
- A host with `[notify] client = true` (a client island) emits `client: true`.
  Its notices carry only the run or orchestrator name and state, with no
  text, PR or board link, wherever they are delivered.

Every host keeps an outbox at `~/.hev/attention/outbox/` (`HEV_ATTENTION_DIR`),
one record per notice ID.

## Hooks

Handlers are the same `hev-hook-NAME` executables on PATH as harness hooks,
with the same discovery, `[hooks] timeout` budget, kill switch and
fail-open rules. They get:

- the event (`needs-you`, `ready` or `done`) as their argument and in
  `HEV_HOOK_EVENT`
- `HEV_HOOK_HARNESS=kit`
- the notice JSON on stdin

Their output is ignored. A handler runs at most once per notice ID, on the
host that emits the notice. Failures go to `~/.hev/hooks/warnings.log`.

```sh
#!/bin/sh
# hev-hook-desk-bell: ring on anything that needs me
[ "$1" = needs-you ] && printf '\a' > /dev/tty
```

## Slack

```toml
# ~/.hev/config.toml
[notify.slack]
channel = "U0A4L3ENZ39"            # a user ID opens the bot's DM; or #channel / C...
token = "secret://SLACK_BOT_TOKEN"  # resolved through hev secrets; never a value
kinds = ["needs-you"]               # default; add "ready", "done" to widen
# tag = "[test]"                    # optional prefix on every message
```

- `token` must be a reference: `secret://NAME`, `op://…` or `azkv://…`. It
  is read at post time through the local secrets daemon, or read directly
  when there is none. The value is never written anywhere. On a 1Password
  host, `secret://SLACK_BOT_TOKEN` reads item `SLACK_BOT_TOKEN` in the host
  vault and field, even though that NAME is not a portable lowercase neutral
  name.
- Pass a **user ID** as `channel` to reach a person. The bot opens its own DM.
  A DM channel ID (`D…`) copied from another session fails with
  `channel_not_found`.
- Each post is a `chat.postMessage` with unfurling off. The first line is
  `[needs you] NAME on HOST: TEXT`, followed by one line per link, and the
  whole message is under 400 characters.
- Posts are threaded per run (with its steers) and per orchestrator. The
  thread's `ts` is kept beside that state, in `~/dispatch/NAME/slack-thread.json`
  or the orchestrator directory's `slack-thread.json`. If the thread is gone,
  the next post starts a new one.
- Posts are idempotent by notice ID. A post that fails stays queued and
  retries with backoff (1m, 2m, … capped at 1h; failed after 30 attempts).
  Retries run on the next orchestrator wake, on every hevd scan cycle and
  whenever a new notice is emitted. A crash in the moment between Slack
  accepting a post and the outbox recording it can repeat that one post.
- A failed post never fails a run or a round. The failure shows in:
  - `hev s`: a `notify:` line with counts and the last error
  - `hev dispatch ls`: the ERROR column
  - `hev dispatch show NAME`: the attention notices section
  - `hev orchestrate show NAME`: `attention.json`

### Pull-route hosts

A host posts its own notices whenever it has `[notify.slack]`, whatever its
`notify_route`. `pull` only concerns reaching the owner's tmux. A host without
Slack config (win-oven) keeps pull. Owner receipts carry the notice, so
these commands re-emit it on a host that has Slack config:

- `hev orchestrate notices pull --host H --deliver`
- `hev dispatch notices pull --deliver`
- teammate member-notice pulls

The pulling host's alias names the host. The stable ID means a redelivered
receipt does not post twice.

## Per-host setup

| Host | Config |
| --- | --- |
| laptop, mini, oven | `[notify.slack]` as above |
| win-oven (BCC island) | none. It keeps `notify_route: pull`, and `[notify] client = true` keeps pulled notices to name and state |
