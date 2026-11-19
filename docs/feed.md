# The Slack feed

The feed posts what agents on a host are doing to one Slack channel, the way
a colleague would: one thread per session, dispatch run or orchestrator, and
in it a sentence or two per turn written by a small model. Attention notices,
feedback-board posts and acceptance evidence land in the same channel. It
replaces the `hev-hook-slack` script that used to live in hev/shell.

It is separate from [`[notify.slack]`](notify.md), which DMs a person when a
notice needs them. A host can have both.

## What it posts

| Event | Posted |
| --- | --- |
| `session-start` | A dispatch run: one sentence from its order ("Started kit-pick-workspaces: adding workspace rows to the picker."). An interactive session: "Session started in `~/workspace/kit`." Resume, compact and clear say so. |
| `stop` | One or two sentences on what the turn did, from the prompt and the agent's reply in the transcript. |
| orchestrator `stop` | `Round N:` and one sentence, in the orchestrator's single thread. An orchestrator's session starts are not posted. |
| `needs-you`, `ready`, `done` | The notice, in the run's or orchestrator's thread. A decision is `decision KEY: QUESTION` with the question verbatim (read back from the run's `decisions.json` when the notice cut it), then the PR and `hev dispatch show` lines. |
| `board-post` | A thread per board thread on the configured boards: board, number, title, author, a one-sentence summary and the link; replies go into it. The post's images are attached. |
| `evidence` | An acceptance-evidence recording: verdict, expected, actual, revision and the Studio link, with the still frame and the video attached. |
| `prompt-submit`, `pre-tool`, `post-tool`, `pre-compact`, `peer-message` | Off by default. When enabled, one compact line each. |

Threads:

- A dispatch run is one thread (`run:HOST/NAME`) across its sessions and
  reworks, and its attention notices share it. The first post names the run,
  the harness, the host, the model and the working directory.
- An orchestrator is one thread (`orchestrator:HOST/NAME`) for all of its
  rounds. Rounds no longer open a thread each.
- Any other session is one thread per harness session ID.
- A board thread is one Slack thread; an evidence record outside a run is its
  own thread.

The thread map is `~/.hev/feed/threads.json` (the newest 1000). A thread
deleted in Slack is started again on the next post.

## Summaries

Summaries come from the Anthropic Messages API, called directly with
`model` (default `claude-haiku-5-5`, low effort, a 20 second timeout). Kit
never runs `claude -p` for this: a harness would fire kit's hooks and the feed
would post about itself.

- The model sees the turn's prompt (its last 4000 characters) and what the
  agent wrote after it (the last 8000), or a run's order (the first 6000), an
  orchestrator round's `reply.md`, or a board post's body. Tool calls and tool
  output are not sent.
- Everything bound for the model or Slack goes through kit's redaction
  (`internal/redact`, the gitleaks rules) first, and the model's answer is
  redacted again.
- Claude Code can run the Stop hook before the turn's last reply is in the
  transcript. The sender waits until a stop is 3 seconds old, reads past the
  size the hook saw, and stops at the next prompt, so a later turn never leaks
  into this one.
- On a model error, a timeout, a refusal or no `model_key`, the post is a
  compact line instead: "Finished a turn: <the reply's last paragraph's first
  sentence>", "Round N: <the first line of reply.md>", or the order's title.
  Nothing is dropped. `~/.hev/feed/sender.log` records why.

## Client work

A session is client work when its directory is under a `client_roots` entry
(default `~/workspace/clients`), when it is a dispatch run whose repository
is, or when the host has `[notify] client = true`. Its records are cut before
they are spooled: no prompt, reply, transcript path, command or working
directory is written, nothing goes to the model, and nothing is uploaded.
Slack gets "Client session active." and "Client session: turn ended.", and a
client notice only its state. Evidence from client work is not posted.

## Attachments

Evidence stills (`final.png`, else `poster.jpg`), `recording.mp4`, and
board-post images are uploaded into the thread with Slack's
`files.getUploadURLExternal` and `files.completeUploadExternal`. That needs
the bot token's `files:write` scope.

- A file over `max_upload_mb` (default 50) is not uploaded. The thread gets
  ":paperclip: NAME not attached (size, over the cap): LINK", with the Studio
  or board link.
- Without `files:write` (`missing_scope`), every file gets the same line
  with "the bot token lacks files:write", once the first upload fails.
- An upload failure never drops the message it belongs to.

## Config

```toml
# ~/.hev/config.toml
[feed]
channel = "C0A6L0UFU6R"                                   # channel ID or #name; the bot must be a member
token = "op://layer-factory/SLACK_BOT_TOKEN/credential"   # bot token: a secret reference, never a value
model_key = "op://layer-factory/hevmind-claude/credential" # Anthropic key; omit for compact lines only
# model = "claude-haiku-5-5"
# enabled = true                     # a [feed] table is on unless this says false
# events = ["session-start", "stop", "needs-you", "ready", "done", "board-post", "evidence"]  # the default
# skip = []                          # removed from events
# boards = ["feedback"]              # hev board boards that feed the channel
# client_roots = ["~/workspace/clients"]
# max_upload_mb = 50
# tag = "[test]"                     # prefixes each thread's first post
```

- `token` and `model_key` must be `secret://NAME`, `op://…` or `azkv://…`.
  They are read through hev secrets in the sender, never in the hook, and
  never written anywhere.
- `events` takes `session-start`, `prompt-submit`, `pre-tool`, `post-tool`,
  `stop`, `pre-compact`, `needs-you`, `ready`, `done`, `peer-message`,
  `board-post` and `evidence`. An unknown name, or a token value, turns the
  feed off and `hev feed status` says why.
- `HEV_FEED=off` turns it off for one process tree. `HEV_HOOKS=off` and
  `[hooks] enabled = false` turn it off with the rest of the hook layer.
- `HEV_FEED_DIR` moves its state from `~/.hev/feed`.
  `HEV_SLACK_API_URL` and `HEV_ANTHROPIC_API_URL` point the sender at fake
  servers in tests.

## How it runs

The feed is a built-in hook handler. It costs the hook almost nothing: it
reads `[feed]`, writes one record to `~/.hev/feed/spool/`, and starts
`hev feed send` in its own session when none is running. Attention notices and
board posts reach it through the same run-lifecycle path as `hev-hook-NAME`
handlers ([hooks](hooks.md)), once per notice on the host that emits it.

`hev feed send` holds `~/.hev/feed/sender.lock`, posts records oldest first
about one a second, and exits after 45 idle seconds. Each record is moved to
`sending/` before it is posted, so no two senders post it.

- A bad token, a missing channel, rate limiting that does not clear, or a
  refused connection pauses the sender (10 minutes, or 1 minute for a
  connection) and keeps the record. hevd's scan cycle restarts the sender.
- A message Slack refuses, or one whose request reached Slack without an
  answer, is dropped and logged, never posted twice.
- The spool keeps at most 2000 records; the oldest go first.

Acceptance evidence is noticed from the recorder itself: when a hooked
session's shell command runs the acceptance recorder
(`node …/record.cjs FLOW OUT_DIR [STUDIO]`), the feed queues `OUT_DIR` and
the sender posts its `result.json` after the command finishes. Nothing outside
kit has to change. For a recording made elsewhere:

```sh
hev feed evidence ~/jobs/my-task/evidence-001 --run kit-pick-workspaces
```

## Commands

```sh
hev feed status                       # config, spool, pause and recent sender log
hev feed evidence DIR [--run NAME]    # post a recording directory
hev feed replay FILE --print          # run recorded events through the feed and print the posts
hev feed replay FILE --fake-token     # ... to HEV_SLACK_API_URL, a fake Slack
```

`replay` reads hook events (`~/.hev/hooks/events.jsonl` lines), attention
notices or outbox records (`~/.hev/attention/outbox/*.json`), board-post events
and `{"evidence_dir": DIR, "run": NAME}` lines, with every event kind enabled
except tool calls, in a scratch state directory. A stop's transcript is read
as it was at the event's time.

## Moving off hev-hook-slack

1. Add `[feed]` to each host's config (laptop, mini, oven) from hev/shell's
   host config.
2. Remove `hev-hook-slack` from `~/.local/bin`, or both will post.
3. `hev feed status` on each host, then watch one turn reach the channel.
