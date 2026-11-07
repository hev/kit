# OpenCode capture

Kit reads OpenCode's own session database with `harness=opencode`. No export,
plugin or OpenCode configuration change is needed. `hev up` (daemon) and
`hev index` both scan it alongside Claude Code, Codex, PI and dot.

## Where OpenCode keeps sessions

OpenCode 1.2 and later (checked against 1.19.5, commit `810c0eb`) stores
sessions in SQLite, `opencode.db`, inside its data directory. That directory comes from
[`xdg-basedir`](https://github.com/sindresorhus/xdg-basedir) 5.1.0 (pinned in
`packages/core/package.json`), which uses the same rule on every OS:

| OS | Default data directory |
| --- | --- |
| Linux | `~/.local/share/opencode` |
| macOS | `~/.local/share/opencode` (not `~/Library/Application Support`) |
| Windows | `%USERPROFILE%\.local\share\opencode` |

`XDG_DATA_HOME` replaces `~/.local/share` on all three. Non-release builds
write `opencode-<channel>.db` beside `opencode.db`; Kit reads both. OpenCode's
`OPENCODE_DB` can point at any other file. Kit does not follow it, so set
`HEV_OPENCODE_DATA_DIR` to that file's directory, and name the file
`opencode.db` or `opencode-<name>.db`. There is no `OPENCODE_DATA_DIR` setting
in 1.19.5.

Sources at v1.19.5: data directory in `packages/core/src/global.ts`; database
file name, `OPENCODE_DB`, channel names and pragmas in
`packages/core/src/database/database.ts`; tables in
`packages/core/src/session/sql.ts`; message and part shapes in
`packages/schema/src/v1/session.ts`; Windows path storage in
`packages/core/src/database/path.ts`.

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

OpenCode keeps the database open in WAL mode. Kit never writes it and never
copies it. Each scan opens a new `mode=ro` connection with `query_only`,
reads one session inside a single read transaction, and closes the connection
before moving on. A WAL reader never blocks OpenCode's writer and only sees
committed rows. No lock or snapshot lasts past one session read, so checkpoints
are not held back between scans. OpenCode's own `busy_timeout` is 5 s, and Kit
uses the same value for the rare moment when the WAL index is being rebuilt.

Each session is one capture unit, keyed `<database>#<session id>`. Its change
stamp is the session's `time_updated` plus the count, newest and summed
`time_updated` of its message and part rows. A session that gains turns, or
streams into a part in place, is read again whole. Turn IDs are OpenCode's
message and part IDs, so the archive upserts the same chunks again and adds
only the new ones. Unchanged sessions are skipped without being read.

## What is captured

- User prompts (text parts). Attached files become a `[file omitted: name]`
  marker; their data is not copied.
- Context OpenCode injects into a user message (`synthetic` text, such as the
  contents of an attached file) becomes a separate `system` turn, so it never
  counts as the user's request.
- Assistant text, reasoning (as `thinking`) and tool calls with their full
  input. Each finished call's output or error is a `tool` turn with the call ID
  and error flag. Pending or running calls appear as calls with no result yet.
- Model, effort (the model variant when not `default`), working directory,
  timestamps, token usage and OpenCode's own cost per assistant message.
  Reasoning tokens are added to output tokens. When OpenCode records no cost,
  the dashboard prices the request from Kit's rate table.
- The session title, as the trace summary.
- Subagent (task tool) runs are their own OpenCode sessions. They are captured
  as side-chain sessions whose agent ID is the child session and whose parent
  agent ID is the parent session.
- Assistant errors (for example aborted or API errors), as a text block.
- Skill loads: OpenCode's `skill` tool (`{"name": …}`), `read` of a
  `skills/…/SKILL.md` (`filePath`) and shell reads, as described in
  [skill capture](skill-capture.md). The `skill` tool records only the name;
  the directory OpenCode reports in the tool's result is not evidence of the
  call, so the load has no path.

Secret scrubbing and every other archive guarantee apply exactly as for the
other harnesses. Scrubbing happens before any write.

## What is not captured

- Step markers, snapshots, patch summaries (file lists and git hashes),
  compaction markers and retry records. The edits themselves are visible as the
  `edit`/`write`/`patch` tool calls.
- Attachment and image contents, MCP resource bodies, todo lists, permissions,
  share URLs, and the project, workspace, account and credential tables.
- Sessions stored only in OpenCode's newer `session_message` (v2) table. 1.19.5
  does not write it in normal use. If a session has v2 rows and no v1 messages,
  the scan reports an error for that session rather than archiving it empty.
- The JSON file store from before OpenCode 1.2 (`storage/session/…`). I found
  no code in 1.19.5 that imports it into `opencode.db`, so sessions that exist
  only there are not captured.

Malformed rows (invalid JSON, unknown part types or roles) reject the whole
session for that scan. The error names the row ID and never echoes its content.
The session is retried when it changes.

## Validation status

- Linux: checked against real sessions from OpenCode 1.19.5 (npm `opencode-ai`),
  captured by a local `hev up` stack and found with `hev traces search`.
- macOS and Windows: data directory and Windows path encoding checked from
  source only. Windows directory strings (`C:/…`, `//server/…`) are converted
  back to native separators and covered by unit tests. Kit's Windows build is
  vetted and its tests compile. Nothing was run on a real Mac or Windows OpenCode.

## Fixtures

`internal/trace/testdata/opencode/opencode-1.19.5.sql` is a dump of real
OpenCode 1.19.5 sessions run against the free `opencode/big-pickle` model in a
throwaway `/tmp/ocproj` project. It holds five sessions plus one subagent child,
covering a skill load, bash, read, a failed read, a task subagent, an attached
file and an edit. It contains no personal paths or secrets. Tests build a WAL
database from it, then add rows to cover growth, damage and Windows paths.
