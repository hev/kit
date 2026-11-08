# OpenCode capture

Kit reads OpenCode's own session database with `harness=opencode`. No export,
plugin or OpenCode configuration change is needed. `hev up` (daemon) and
`hev index` both scan it alongside Claude Code, Codex, PI and dot.

## Where OpenCode keeps sessions

OpenCode stores sessions in SQLite, `opencode.db`, inside its data directory.
Kit reads both current layouts:

- **OpenCode 2.x** (checked against 2.2.6, commit `dc2ffcf`, released
  2026-11-06): tables `session_v2` and `session_message`, with assistant parts
  inline in the message or, when `experimental.part_rows` is on, in
  `session_part` rows.
- **OpenCode 1.2 to 1.19.x** (checked against 1.19.5, commit `810c0eb`): tables
  `session`, `message` and `part`.

The data directory and file name did not change in 2.x, and the 2.x background
service (`opencode serve --service`) is started by the CLI with the same
environment, so it writes the same database. The directory uses the same rule
on every OS (1.x through
[`xdg-basedir`](https://github.com/sindresorhus/xdg-basedir) 5.1.0, 2.x through
its own `packages/util/src/global-roots.ts`):

| OS | Default data directory |
| --- | --- |
| Linux | `~/.local/share/opencode` |
| macOS | `~/.local/share/opencode` (not `~/Library/Application Support`) |
| Windows | `%USERPROFILE%\.local\share\opencode` |

`XDG_DATA_HOME` replaces `~/.local/share` on all three. Non-release builds
write `opencode-<channel>.db` beside `opencode.db`; Kit reads both. OpenCode's
`OPENCODE_DB` can point at any other file. Kit does not follow it, so set
`HEV_OPENCODE_DATA_DIR` to that file's directory, and name the file
`opencode.db` or `opencode-<name>.db`. Neither 1.19.5 nor 2.2.6 has an
`OPENCODE_DATA_DIR` setting.

Sources at v2.2.6: data directory in `packages/util/src/global-roots.ts`;
database file name, `OPENCODE_DB` and channel names in
`packages/cli/src/database-path.ts`; pragmas in
`packages/core/src/database/database.ts`; tables in
`packages/core/src/session/sql.ts`; message shapes in
`packages/schema/src/session-message.ts`; part rows in
`packages/core/src/session/projector.ts`; the 1.x upgrade in
`packages/core/src/database/v1-migration.bun.ts`. Sources at v1.19.5: data
directory in `packages/core/src/global.ts`; database in
`packages/core/src/database/database.ts`; message and part shapes in
`packages/schema/src/v1/session.ts`. Windows path storage is in
`packages/core/src/database/path.ts` in both.

### Upgrading from 1.x to 2.x

On first start, 2.x copies every 1.x session into its own tables under the same
session and message IDs, adds backdated `system` messages (tool rename notices)
and `synthetic` messages of its own, and leaves the 1.x rows in place. Kit keeps
reading such a session's 1.x rows exactly as before and adds only the 2.x
messages written after its last 1.x message. Turn IDs are unchanged, so an
upgrade adds nothing to the archive until the session continues. A pre-release
`opencode-next.db` (2.x imports it into `opencode.db`) is not read on its own.

Kit picks the root in this order:

1. `HEV_OPENCODE_DATA_DIR` (highest priority)
2. `[capture] opencode_data_dir` in Kit configuration
3. `$XDG_DATA_HOME/opencode`, else `~/.local/share/opencode`

```sh
hev index --read-side --tier all
hev traces search --harness opencode 'distinctive phrase'
hev traces show <session id>
```

## Reading while OpenCode runs

OpenCode keeps the database open in WAL mode. Kit never writes the database
file and never copies it. Each scan opens a new connection with `query_only`,
reads one session inside a single read transaction, and closes the connection
before moving on. No lock or snapshot lasts past one session read, so
checkpoints are not held back between scans.

- **OpenCode running** (an `opencode.db-wal` file exists): Kit opens with
  `mode=ro` as an ordinary WAL reader. It shares OpenCode's own `-wal` and
  `-shm` files, which SQLite readers must map, never blocks OpenCode's writer
  and only sees committed rows. OpenCode's own `busy_timeout` is 5 s, and Kit
  uses the same value for the rare moment when the WAL index is being rebuilt.
- **OpenCode closed** (no `-wal`; OpenCode removes it on a clean exit): a
  `mode=ro` open would still create `-wal` and `-shm` files, and fail in a
  read-only directory. Kit opens with `immutable=1` instead, which creates
  nothing. Because `immutable` is only safe while nobody writes, Kit keeps the
  result only if the database's size and modification time are unchanged and
  no `-wal` appeared during the read; otherwise OpenCode started meanwhile and
  the read is repeated as a WAL reader.

One database that cannot be read (corrupt, or unreadable for any reason) is
reported as an error each scan without hiding sessions in the other databases.
So is a database whose tables match neither layout, which is what a future
OpenCode storage change would look like: the error ("no OpenCode session layout
Kit recognises") shows as the daemon's last error in `hev s` and in the
`hev index` report, and capture is never silently empty. A channel database
OpenCode created but never migrated (no tables yet) holds nothing and is
skipped.

Each session is one capture unit, keyed `<database>#<session id>`. Its change
stamp is the session's `time_updated` plus the count, newest and summed
`time_updated` of its message and part rows, in each layout it has. A session
that gains turns, or streams into a message or part in place, is read again
whole. Turn IDs are OpenCode's message IDs; a tool result is the 1.x part ID,
or in 2.x the message ID and call ID. The archive upserts the same chunks again
and adds only the new ones. Unchanged sessions are skipped without being read.

Chunk IDs hash their text, so Kit never archives a message OpenCode is still
writing: a truncated copy would stay in the archive next to the final one, and
a secret cut off mid-stream (a private key before its `END` line) could slip
past redaction. A session is captured up to its first assistant message without
`time.completed`; the rest follows on the scan after OpenCode completes it.
Aborted and failed replies are completed too. If OpenCode is killed mid-reply
the message never completes, so once the session has been idle for an hour the
message is captured as it stands. Tool calls whose input is still being
written (`pending` in 1.x, `streaming` in 2.x) are skipped until they start
running.

Instruction capture (`[capture] instructions`) finds project instruction files
through session working directories. It keeps each session's directories by
change stamp, so it reads only sessions that changed since the last cycle, and
a session it cannot read loses only its own projects.

## What is captured

- User prompts (text parts, or the 2.x prompt text) and `@agent` mentions.
  Attached files become a `[file omitted: name]` marker; their data is not
  copied. (`opencode run -f` in 2.x inlines a text file into the prompt text
  itself, so that text is captured as part of the prompt.)
- Context OpenCode injects for the model (1.x `synthetic` text parts; 2.x
  `synthetic` and `system` messages, such as tool rename notices) becomes a
  separate `system` turn, so it never counts as the user's request.
- 2.x shell commands the user runs with `!` are a `shell` call on a user turn
  plus a `tool` turn with the output (an error when the exit code is non-zero
  or the command timed out or was killed).
- Assistant text, reasoning (as `thinking`) and tool calls with their full
  input. Each finished call's output or error is a `tool` turn with the call ID
  and error flag. Running calls appear as calls with no result yet.
- Model, effort (the variant of the message, or of the prompt a reply answers,
  when not `default`), working directory,
  timestamps, token usage and OpenCode's own cost per assistant message.
  Reasoning tokens are added to output tokens. When OpenCode records no cost,
  the dashboard prices the request from Kit's rate table.
- The session title, as the trace summary.
- Subagent (1.x `task`, 2.x `subagent` tool) runs are their own OpenCode sessions. They are captured
  as side-chain sessions whose agent ID is the child session and whose parent
  agent ID is the parent session.
- Assistant errors (for example aborted or API errors), as a text block.
- Skill loads: OpenCode's `skill` tool (1.x `{"name": …}`, 2.x `{"id": …}`),
  `read` of a `skills/…/SKILL.md` (1.x `filePath`, 2.x `path`) and shell reads
  (1.x `bash`, 2.x `shell`), as described in [skill capture](skill-capture.md).
  In 2.x a skill attached to the prompt and a `skill` message also count, as a
  `system` turn holding a `skill` call. The `skill` tool records only the name;
  the directory OpenCode reports in the tool's result is not evidence of the
  call, so the load has no path. As for other harnesses, a call counts even
  when it fails (for example an unknown skill name).

Secret scrubbing and every other archive guarantee apply exactly as for the
other harnesses. Scrubbing happens before any write.

## What is not captured

- Step markers, snapshots, patch summaries (file lists and git hashes),
  compaction markers and summaries (they restate earlier turns), retry records,
  and 2.x idle, agent, model and location switch markers. The edits themselves are visible as the
  `edit`/`write`/`patch` tool calls.
- Attachment and image contents, MCP resource bodies, todo lists, permissions,
  share URLs, and the project, workspace, account and credential tables.
- 2.x skill text attached to a prompt (only the skill name is kept), and tool
  result files (a `[file omitted: …]` marker instead).
- The JSON file store from before OpenCode 1.2 (`storage/session/…`). I found
  no code in 1.19.5 or 2.2.6 that imports it into `opencode.db`, so sessions
  that exist only there are not captured.

Malformed rows (invalid JSON, unknown part types, message types or roles) reject the whole
session for that scan. The error names the row ID and never echoes its content.
The session is retried when it changes.

## Validation status

- Linux: checked against real sessions from OpenCode 2.2.6 (GitHub release
  binary) and 1.19.5 (npm `opencode-ai`), including a real 1.19.5 store
  upgraded by 2.2.6 and a session continued after the upgrade, captured by a
  `hev up` stack and found with `hev traces search`.
- 2.x `session_part` rows, `skill` and `shell` messages and prompt skill
  attachments did not occur in the real sessions (part rows are behind
  `experimental.part_rows`; the others need the TUI). They are covered by unit
  tests built from the source shapes.
- macOS and Windows: data directory and Windows path encoding checked from
  source only. Windows directory strings (`C:/…`, `//server/…`) are converted
  back to native separators and covered by unit tests. Kit's Windows build is
  vetted and its tests compile. Nothing was run on a real Mac or Windows OpenCode.

## Fixtures

`internal/trace/testdata/opencode/opencode-2.2.6.sql` is the 1.19.5 store below
after a real OpenCode 2.2.6 upgraded it, with one of its sessions continued
under 2.2.6, plus six fresh 2.2.6 sessions (a skill load, a failed skill load,
shell, read, edit, a failed read, an attached file, a subagent with its child
session). Only the session tables are kept; paths are `/tmp/ocproj`.

`internal/trace/testdata/opencode/opencode-1.19.5.sql` is a dump of real
OpenCode 1.19.5 sessions run against the free `opencode/big-pickle` model in a
throwaway `/tmp/ocproj` project. It holds five sessions plus one subagent child,
covering a skill load, bash, read, a failed read, a task subagent, an attached
file and an edit. It contains no personal paths or secrets. Tests build a WAL
database from it, then add rows to cover growth, damage and Windows paths.
