# RFC 0013: Hooks and steering — one hook layer across harnesses, and steering that reaches a busy agent

Status: Implemented (2026-11-17) for Claude Code and Codex. pi has no verified
hook mechanism on our hosts yet and gets the turn-boundary fallback. Proposed
2026-11-17. (Number 0013 is shared with [swarms](0013-swarms.md), as 0012 is
shared three ways; the slug disambiguates.)

## Summary

Kit installs one thin hook per harness lifecycle event. Every hook runs
`hev hook EVENT --harness H`, which reads the harness's hook JSON from stdin,
normalizes it to one versioned event (`schemas/hook_event.schema.json`),
appends a redacted copy to the hook event log, runs handlers under a short
budget, and answers in the harness's own shape. Handlers are built in (the
peers inbox and steering) or user executables named `hev-hook-NAME` on PATH.
Every failure answers "continue".

On that layer, `hev steer TARGET MESSAGE --level note|urgent|halt|redirect`
is one front door for steering a peers session, a dispatch run or an
orchestrator. `urgent` reaches an agent at its next tool call instead of its
next turn, and blocks mutating tool calls until the agent runs
`hev steer ack ID`. `halt` ends the turn. `redirect` ends the turn,
checkpoints the worktree, and reworks the same run with the new instruction.
Every steer message has an ID and a receipt that moves
`queued -> injected (time, turn, tool call) -> acked`.

### The decision in five lines

1. One entry point, `hev hook EVENT --harness H`, with six events: `session-start`, `prompt-submit`, `pre-tool`, `post-tool`, `stop`, `pre-compact`.
2. Fail open always: a handler error, panic or overrun of the 2s budget answers "continue" and logs a warning. `HEV_HOOKS=off` and `[hooks] enabled = false` turn the hook into a no-op before any other work.
3. Dispatch and orchestrator rounds wire the hooks per invocation (Claude `--settings`, Codex `-c hooks.*` with `--dangerously-bypass-hook-trust`), so steering a run never depends on a host's global config. `hev hooks install` adds the same hooks to `~/.claude/settings.json` and `~/.codex/hooks.json` for interactive sessions, merging and stamping kit's entries.
4. Blocking exists only to land steering. The pre-tool hook blocks only while an `urgent` is unacknowledged (mutating calls only) or a `halt` is pending (every call). Nothing is ever killed to deliver.
5. Receipts replace "queued N". `dispatch ls/show`, `orchestrate show` and `peers` show counts by state. Reading an inbox file is never an acknowledgement.

## Motivation

- **Board thread #292.** A Codex worker sat in a single long turn with
  `turns0 / toolcalls2389 / queued211`. A scope correction sent through the
  inbox stayed unseen for 4.5 hours while superseded CI rebuilt. Repeated sends
  reported "queued; it is the next turn". The thread asks for exactly this:
  a durable urgent notice at the next safe tool boundary, an acknowledgement
  before another mutation, ordinary follow-ups left queued, no kill or
  restart, and measured delivery latency.
- **Steering waits for turn boundaries.** `dispatch say` and `orchestrate say`
  append to `inbox.md`, which workers read "between steps; delivery does not
  interrupt an active step" (`docs/dispatch.md`). A headless worker's whole
  run is often one turn. Its next turn is a rework.
- **"Queued" says nothing.** A count of queued messages cannot tell a message
  the agent read from one it never saw. #292's manual inbox reads did not
  count, and should not.
- **Hooks are scattered.** The only hook kit installs today is `peers
  runtime`'s UserPromptSubmit `inbox-hook`, wired per launch. Every new hook
  would be another one-off, another harness-specific output shape and another
  way for a broken kit to stop an agent.
- **Adam works in yolo mode.** Permissions are bypassed on purpose. A hook
  layer must never become a permission system; it may block a tool call only
  to make an agent read something it was told.

## Decision

Add `hev hook`, `hev hooks install|check|uninstall` and `hev steer`, a
normalized hook event schema, and a receipt store per steering target.

## Goals

- An urgent message reaches a busy agent at its next tool boundary, exactly
  once, and mutating work waits for its acknowledgement.
- A halted agent ends its turn on its own; the run shows `halted`.
- One hook entry point and one event shape for Claude Code and Codex, with
  user handlers that do not have to know either harness.
- Hook failure is invisible to the agent except as lost steering, and loud in
  the warning log.
- Install merges into user settings without touching anyone else's entries,
  and is idempotent, checkable and removable.

## Non-goals

- A permission or policy system. Hooks do not judge commands beyond the
  mutating-call heuristic that holds an unacknowledged urgent.
- Steering across hosts to a peers session. Remote peers sessions keep
  `peers send` (note); urgent and halt to them are refused with a reason.
- Changing `peers send` receipts or the peers inbox wire format.
- Shipping hook events to Layer. The event log is local; capturing it in the
  trace archive is an open question.
- pi hooks. pi is not installed on our hosts and its extension API was not
  verified; it gets the fallback.

## Vocabulary

- **Hook event**: one normalized lifecycle record (`hev.hook.event/v1`).
- **Handler**: code that receives an event and returns a response:
  `continue`, `block` with a reason, `halt` with a reason, and optional
  context to inject.
- **Tool boundary**: a pre-tool or post-tool hook firing.
- **Target**: what a steer message is for: a dispatch run, an orchestrator
  (`o-NAME`) or a local peers session.
- **Receipt**: a steer message's record and state: `queued`, `injected`
  (time, turn, tool-call index, event), `acked` (time, by whom or "turn
  ended").
- **Delivery**: `tool-boundary` (hooks are wired for the target) or
  `turn-boundary` (fallback; inbox at the next turn, and the harness's own
  interrupt for halt). `hev steer` prints which a target gets.

## Proposed surface

### `hev hook EVENT --harness H`

Events: `session-start`, `prompt-submit`, `pre-tool`, `post-tool`, `stop`,
`pre-compact`. Harnesses: `claude`, `codex`. Native names map one to one
(`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`,
`PreCompact`); Codex 0.162 sends Claude-compatible JSON with `turn_id` added
and tool names `Bash` and `apply_patch`.

Order of work, all inside one deadline (`[hooks] timeout`, default `2s`):

1. Kill switch: `HEV_HOOKS=off|0|false` or `[hooks] enabled = false` in
   `~/.hev/config.toml` exits 0 with no output before reading stdin.
   A settings-installed hook also steps aside when the process was launched
   with invocation hooks (`HEV_HOOK_INVOCATION=1`), so a run never handles an
   event twice.
2. Normalize stdin to the event schema. Unparseable input: warn, continue.
3. Append a redacted event (no prompt text, tool input or tool output; their
   sizes only) to `~/.hev/hooks/events.jsonl`.
4. Run handlers concurrently. Built-in: `peers-inbox` (prompt-submit, only
   in sessions `peers runtime` bound, as `inbox-hook` did) and `steer`.
   User: every `hev-hook-NAME` on PATH, discovered like plugins
   (`docs/plugins.md`), given the event JSON on stdin and expected to print a
   response JSON (`{"decision":"continue|block|halt","reason":"...","context":"..."}`)
   or nothing.
5. Merge: halt beats block beats continue; contexts concatenate in handler
   order. A handler that errors, panics, exits non-zero, prints invalid JSON or
   misses the deadline contributes nothing and logs a warning to
   `~/.hev/hooks/warnings.log`.
6. Render natively and always exit 0: `additionalContext` for
   prompt-submit, session-start and post-tool; `permissionDecision: "deny"`
   with the reason for a pre-tool block; `continue: false` plus `stopReason`
   for halt.

`hev peers inbox-hook` keeps its exact output (it is checked byte for byte
against the Python peers); its logic is shared with the `peers-inbox` handler.

### `hev hooks install|check|uninstall [--harness claude|codex]`

- Claude Code: `~/.claude/settings.json` `hooks`, one entry per event.
  Codex: `~/.codex/hooks.json`. Each kit command ends in
  `--installed-by kit`, the stamp `check` and `uninstall` look for.
- Install merges: foreign entries, foreign keys and other settings are kept
  byte-for-byte in meaning; kit entries are replaced in place; a second
  install is a no-op. Uninstall removes only stamped entries and empty event
  arrays it leaves behind.
- `check` exits non-zero when a harness home exists and kit's entries are
  missing or point at another binary. Codex also needs a one-time trust
  review (`/hooks` in the TUI) before a global hook runs; `check` says so.
- `hev skills sync` and `hev update` repair kit's entries where they are
  already installed; they never install hooks that were not asked for.

Dispatch and orchestrator rounds do not need any of this: their supervisors
wire the same commands per invocation and set `HEV_STEER_DIR`.

### `hev steer TARGET MESSAGE... [--level L] [--host H] [--halt-timeout D]`

| Level | Delivery with hooks | Fallback (no tool-boundary hook) |
| --- | --- | --- |
| `note` (default) | injected at the next prompt-submit; dispatch/orchestrate also append `inbox.md` as `say` does | `say` / `peers send` as today |
| `urgent` | injected once at the next tool boundary (post-tool context, or the block reason if the next call is mutating); mutating calls blocked until `hev steer ack ID` | appended to `inbox.md` (turn boundary) |
| `halt` | every tool call blocked with "end your turn now" and `continue: false`; the run becomes `halted` | the harness's own interrupt (SIGINT to a headless harness, Escape to a peers pane) |
| `redirect` | halt, then a checkpoint commit of the worktree and a rework of the same run with the instruction appended (branch and PR kept) | same, after the interrupt |

- `dispatch say`, `orchestrate say` and `peers send` keep working and are
  `note`.
- Mutating calls: `Write`, `Edit`, `MultiEdit`, `NotebookEdit`,
  `apply_patch`; shell commands running `git commit|push|merge|rebase|reset|…`,
  `gh pr create|merge|edit|comment|…`, `gh api` with a writing method,
  `hev dispatch` other than reads, file writers (`rm`, `mv`, `cp`, `tee`,
  `sed -i`, …) or output redirection other than to `/dev/null`. `hev steer`
  is always allowed. Reads are always allowed.
- A halt that has not ended its turn within `--halt-timeout` (default
  `[hooks] halt_timeout`, `10m`) escalates to a hard stop by the dispatch
  supervisor, and the receipt says so.
- `hev steer ack ID...` acknowledges injected messages (an un-injected
  message is refused: the agent cannot have been told it). `hev steer ls
  [TARGET]` and `hev steer show ID` print receipts.

### Receipts

One JSON file per message under the target's steer directory:
`~/dispatch/NAME/steer/` (run), `~/dispatch/orch-NAME/steer/` (orchestrator),
`~/.hev/steer/peers/SESSION/` (local peers session). Writes are atomic
renames under a directory lock, so concurrent hooks (parallel tool calls)
inject a message once. `state.json` beside them counts turns and tool calls
for the target's current harness session, which is what `injected` records.
`dispatch ls` gains a `STEER` column (`q1 i0 a2`), `dispatch show` and
`orchestrate show` print counts and receipts, `hev peers` appends counts to a
session that has unacknowledged steering, and a halted run's state is
`halting` while its harness exits and `halted` after.

## Open questions

- Should the redacted hook event log feed the trace archive (a `hook` block
  kind), so latency and blocks are searchable beside the transcript?
- Codex global hooks need a persisted trust review per hook. Should
  `hev hooks install --harness codex` offer to record trust, or is that a
  decision only a person makes in the TUI? This RFC leaves it to the person.
- pi: its extension API can probably host the same shim. Verify on a host
  that has pi before adding it.
- Should `urgent` also hold `hev dispatch validate` and long CI waits, not only
  mutations? #292's waste was rebuilt CI; a CI rerun is usually `gh workflow
  run` or a push, which are already held.
- Remote peers sessions: route urgent/halt through the relay once the relay
  carries receipts.
