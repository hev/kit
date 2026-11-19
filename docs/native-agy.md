# Antigravity CLI (agy) capture

Kit reads Google Antigravity CLI conversations from agy's own store, with
`harness=agy`. No hooks, export or agy configuration change is needed. `hev up`
(daemon), `hev index` and `hev traces backfill` scan it alongside Claude Code,
Codex, PI, OpenCode and dot. Checked against agy 1.5.12 on Linux (Oven) and
macOS (the mini).

## Where agy keeps conversations

agy's data directory is `.gemini/antigravity-cli` in the user's home directory
on every OS. The path is fixed in the agy binary; there is no setting or
environment variable that moves it.

| OS | Data directory |
| --- | --- |
| Linux | `~/.gemini/antigravity-cli` |
| macOS | `~/.gemini/antigravity-cli` |
| Windows | `%USERPROFILE%\.gemini\antigravity-cli` |

Set `HEV_AGY_DATA_DIR`, or `agy_data_dir` under `[capture]` in
`~/.hev/config.toml`, to read another directory. In it:

- `conversations/<conversation id>.db`: one SQLite database per conversation,
  in WAL mode. This is what kit reads.
- `conversation_summaries.db`: one row per conversation with its title (agy
  titles interactive sessions; `agy -p` runs have none), preview, workspace
  and parent conversation, plus agy's own full-text search index.
- `brain/<conversation id>/`: agy's working files for the conversation.
  `.system_generated/logs/transcript.jsonl` and `transcript_full.jsonl` are a
  JSON rendering of the steps (no tool call IDs or exit codes),
  `.system_generated/steps/<n>/output.txt` holds a tool's full output,
  `.system_generated/messages/` the system messages delivered to the agent,
  and `scratch/` and `.user_uploaded/` the agent's and the user's files. Kit
  reads none of it.

## What a conversation database holds

| Table | What it is |
| --- | --- |
| `steps` | one row per step: `idx`, `step_type`, `status`, `step_payload` |
| `gen_metadata` | one row per model call: which steps it produced and with which model |
| `trajectory_metadata_blob` | the workspace (folder, git origin, branch) and the parent conversation of a subagent |
| `trajectory_meta`, `executor_metadata`, `parent_references`, `battle_mode_infos` | agy's runtime state; not read |

`step_payload` is a `gemini_coder.Step` protobuf. agy publishes no schema, but
its binary carries the compiled descriptors
(`third_party/gemini_coder/proto/trajectory.proto`,
`third_party/jetski/cortex_pb/cortex.proto`,
`third_party/jetski/codeium_common_pb/codeium_common.proto`), and the field
numbers in `internal/trace/agy_proto.go` are taken from them, each with agy's
name for it. `step_type` is `exa.cortex_pb.CortexStepType` and `status` is
`CortexStepStatus`.

| `step_type` | agy's name | Captured as |
| --- | --- | --- |
| 14 | `USER_INPUT` | a user turn: the prompt as typed, and any images |
| 15 | `PLANNER_RESPONSE` | an assistant turn: thinking, text and tool calls (name, arguments, call ID), with model, effort and token usage |
| 132 | `GENERIC` | a tool turn: the result agy gave the model, paired to its call by ID, with the command's exit status |
| 101 | `SYSTEM_MESSAGE` | a system turn: notices agy injects for the model |
| 17 | `ERROR_MESSAGE` | a system turn: `[error] …` |
| anything else | | skipped, unless it answers a tool call, when it is that call's result |

Every tool seen in agy 1.5.12 sessions (`run_command`, `read_file`,
`write_file`, `edit_file`) records its result as a `GENERIC` step; none of the
typed step types in the enum (`RUN_COMMAND` 21, `VIEW_FILE` 8, ...) were
written. `status` 3 is `DONE` and 7 `ERROR`.

| Kit field | From |
| --- | --- |
| session ID | the database file name (agy's conversation ID) |
| model, effort | `gen_metadata` → `ChatModelMetadata.response_model` (`gemini-4-neon`) and the effort in `model_display_name` (`Gemini 4 Neon (Medium)` → `medium`); the step's own model field is a placeholder enum |
| tokens | the step's `ModelUsageStats`: input, output (thinking included), cache read and cache write |
| request ID | `ModelUsageStats.response_id` |
| timestamps | each step's `created_at` |
| workdir, branch, repo | `trajectory_metadata_blob` workspace (a `file://` URI, turned into a native path, including `C:\…` and UNC paths) |
| title | `conversation_summaries.db` |
| side chain | a parent conversation in `trajectory_metadata_blob` or the summaries |
| exit status | `GenericStepResult.step_render_info` terminal block `exit_code`; a non-zero status, or an `ERROR` step, marks the result as an error |

Thinking is captured as `thinking` blocks, as for every other harness. A
tool result is the text agy gave the model, without agy's `Created At` /
`Completed At` header. For long output that is agy's own truncation notice
and the tail of the output; the whole output stays in agy's
`brain/<id>/.system_generated/steps/<n>/output.txt`, which kit does not read.

## Reading while agy runs

agy holds each conversation's database open in WAL mode for as long as the
session runs. Kit reads it the way it reads OpenCode's database: a fresh
read-only connection with `query_only`, one read transaction per read, closed
before returning, so no lock or snapshot outlives a scan and agy's writes are
never blocked. With no `-wal` file (agy closed), the file is opened with
`immutable=1`, so no `-wal` or `-shm` files are created beside it; this is also
why `sqlite3 -readonly` fails on such a file on macOS ("unable to open database
file") while kit reads it. If SQLite still cannot open a database in place,
kit copies it and its WAL to a private temporary directory, reads the copy, and
keeps the read only if the originals did not change during the copy.

A step agy has not finished (a response still streaming, a command still
running) ends what is read, so the archive only ever gains a stable prefix.
Once a conversation has been idle for an hour, such a step is archived as it
stands.

Each conversation is one unit; its change stamp is the size and modification
time of the database and its WAL, plus the title. Turn IDs are
`<conversation id>:<step index>`, so a conversation that grows is re-read whole
under the same IDs.

## Unknown steps and fields

Decoding is by hand over the protobuf wire format. A field kit does not name
is skipped, whatever its type; a step type kit does not know is skipped (or
read as a tool result if it answers a tool call); a payload that is not valid
protobuf skips that step. None of these fail the read. A database without a
`steps` table is reported as an error each scan, which is what a change of
agy's storage would look like.

## Hooks

agy has Claude-style hooks (`SessionStart`, `PreToolUse`, `PostToolUse`,
`Stop`, ...). Kit does not use them: it captures every harness by scanning the
harness's own store, which also covers sessions from before kit was installed
and sessions run while hevd was down, and the database already holds
everything the hooks would report.

## Fixtures

`internal/trace/testdata/agy/` holds real agy 1.5.12 conversations made on
Oven from harmless prompts in a scratch directory (a plain answer, a shell
command, a file write and edit, a failing command continued with `agy -c`, and
an interactive session with a title and long output), dumped as SQL by
`scripts/generate-agy-fixture.py`, which drops the system prompt, per-user
configuration and opaque signatures. `provenance.json` lists the prompts.
