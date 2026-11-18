# "Needs you" notices

Kit raises one typed **attention notice** for each moment it records, on the
host where it happens. Only two moments are `needs-you`, meaning a person has
to act: an open decision, and a halt that escalated to a hard stop. Every
other end (failed, timeout, blocked, needs-review, an admission failure, a
stall) is a record on the opt-in `done` kind. Each notice:

- runs your `hev-hook-NAME` handlers as a run-lifecycle event
  ([hooks](hooks.md)), and
- posts to Slack when that host has `[notify.slack]`.

The existing peers delivery (dispatch `notify`, orchestrator owner notices,
`notify_route: pull`) is unchanged. Notices are added alongside it.

## When a notice is raised

| Kind | Source | Moment |
| --- | --- | --- |
| `needs-you` | `run` | a dispatch run ends `needs-decision KEY: QUESTION` (an open decision) |
| `needs-you` | `steer` | a steering halt escalates to a hard stop ([steering](steering.md)) |
| `needs-you` | `orchestrator`, `expedite`, `swarm` | a round's `reply.md` first line is `[needs you] needs-decision KEY: QUESTION`, or a legacy `hev expedite round` line has `[needs you] needs-decision KEY: QUESTION` |
| `ready` | `run` | a validation PASS lands on a run whose PR is ready (not draft) with all checks green |
| `ready` | `orchestrator`, … | an orchestrator meets its goal (`goal-met.json`) |
| `done` | `run` | a dispatch run ends `done`, `failed`, `timeout`, `blocked` or `needs-review`, a queued run fails admission, or a repeating run ends (`until`, `max_lifetime`, `repeat removed`) |
| `done` | `orchestrator`, `expedite`, `swarm` | an orchestrator is stopped, an expediter stalls, or a `[needs you]` reply or expedite line is not a decision (a stalled or failed run, a missing tool, a merge to make) |

`state` says which end a `done` notice records (`failed`, `blocked`,
`stalled`, …), and the record stays in `attention.json`, `ls` and `show`
whether or not anything is posted. `done` reaches hook handlers always and
Slack only when `[notify.slack]` `kinds` includes `"done"`; the default
`kinds` is `["needs-you"]`.

A stop someone asked for and a halt that ended its turn in time raise
nothing. Validator children raise nothing. A repeating (`every`) run raises a
notice for each round that does not end `done` and nothing for an uneventful
round; the same round seen again when the repeat ends is the same notice. The
orchestrator reply notice is raised whether or not the orchestrator has
`notify` authority. Authority still gates the peers owner notice.

### Dedupe by subject

A `needs-you` notice's ID is its **subject**, never the attempt or round
that raised it:

| Source | Subject |
| --- | --- |
| run | the run (host, name, first attempt's start), the decision key, and the question |
| steer | the run (as above) and the halt ID (the attempt when the halt has no ID) |
| orchestrator, expedite, swarm | the orchestrator (host, name, first start), the decision key, and the question |
| legacy expedite line | the run (host, name), the decision key, and the question |

Whitespace in the question is collapsed first. Raising `needs-you` again with
an unchanged subject (a rework or a later round that asks the same question,
an orchestrator repeating it every round) is a no-op: no new notice, no hook
run and no Slack post. A changed key, question or halt is a new subject and
raises again. A rework or an amend starts a new attempt but keeps the run's
first start (`lineage` in `meta.json`), and reapplying an orchestrator keeps
its first start (`first_start` in its state), so neither changes the subject.
`done` records keep a per-attempt key (run start and end, or
round number; orchestrator round; expedite line and UTC day).

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
  "thread": "run:oven/kit-needs-you-slack-20261117",
  "requested_by": {"person": "user_2pQx7Lr0aBcDeFgHiJkLmNoP", "name": "Adam"},
  "addressee": ["user_2pQx7Lr0aBcDeFgHiJkLmNoP"]
}
```

- `id` is derived from what the notice is about: source, host, name, kind, and
  the subject for `needs-you` ([dedupe by subject](#dedupe-by-subject)) or
  the attempt for other kinds. It never comes from the emit time. Emitting the same moment again does nothing: not after a retry,
  not after a supervisor restart, and not when a pulled receipt is redelivered.
- `text` is one line, with whitespace collapsed, credentials redacted and at
  most 240 characters. It never holds board bodies or repository content.
- A host with `[notify] client = true` (a client island) emits `client: true`.
  Its notices carry only the run or orchestrator name and state, with no
  text, PR or board link, wherever they are delivered. The cut happens on the
  client host before an owner or member receipt is written, so the private
  text never leaves it, and `client: true` travels with the receipt to the
  host that pulls it. A host whose config cannot be read writes receipts as
  if it were a client island.

### Who a notice is for

Every notice kit emits carries `requested_by` and `addressee`, and so does the
hook event.

- `requested_by.person` is the hosted (Clerk) user ID, `author.id` from
  `GET /v1/whoami`, when the emitting host is signed in to a kit service
  (`HEV_API_KEY`, or the `[kit]` key `hev login` writes) and the requester is a
  local sender. Otherwise it is the run's existing sender: `local:ACCOUNT`, or
  a nodes teammate's name. A teammate's run is always for the teammate.
  `requested_by.name` is the signed-in author's name, else the sender. The
  whoami answer is cached for ten minutes per process. If the call fails, kit
  uses the author that sign-in last recorded, and a host with no key counts as
  logged out.
- The requester is inherited. A rework keeps its run's requester. A validator
  or review takes its source run's. An orchestrator records its applier (or
  the orchestrator or run that applied it) in `state.json`. A run dispatched
  by an orchestrator, or from inside another run's worktree, takes that
  orchestrator's or run's requester.
- `addressee` lists the persons or roles the notice asks, and defaults to
  `[requested_by.person]`.

Neither field changes delivery. Slack posts still go to the host's
`[notify.slack]` channel, and the message text does not include them. Both
fields are additive to `v1`. A receipt from an older host that lacks them is
attributed to the pulling host's account when it is re-emitted.

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
- Posts go out at most once per notice ID. A post that fails before its
  request is sent (the token read fails, the connection is refused) or that
  Slack rejects stays queued and retries with backoff (1m, 2m, … capped at
  1h; failed after 30 attempts). Retries run on the next orchestrator wake,
  on every hevd scan cycle and whenever a new notice is emitted.
- Slack's `chat.postMessage` has no idempotency key, so kit never repeats a
  post Slack may already have. Just before the request goes out, the outbox
  durably records `sending`. If the process stops before the answer is
  recorded, a later sender finds `sending` under an expired lease. If the
  connection drops after the request was written, the outcome is also
  unknown. Either way the post becomes `uncertain`: it is never reposted, and
  it shows in `hev s` (`N uncertain (check Slack; not reposted)`), the `ls`
  ERROR column and `show`. Check the DM or channel. If it is missing, the
  notice is in the outbox record and `attention.json`.
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
