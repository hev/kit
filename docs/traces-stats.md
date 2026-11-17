# Trace stats: capture facts, run kinds and `hev traces stats`

`hev traces stats` answers questions about how sessions behave across the
archive (which skills get loaded and where, how many tool calls a session
spends before its first edit, how often sessions search the board, how often
tool calls fail, how big the starting context is, how many dispatches fail at
launch) without dumping sessions one by one. It reads the archive: Layer with
a key, the file archive without one.

This page covers the capture facts the command reads, the run-kind rule, the
command and its JSON, and the backfill that adds the facts to older sessions.

## Capture facts

Capture version 1 adds these attributes. None is filterable; none holds file
contents, prompt text or secret values. The JSON-valued attributes are stored
as JSON strings, the way `tool_counts` and `skill_loads` already are.

### Session rows (`<namespace>-sessions`)

| Attribute | Type | Meaning |
|---|---|---|
| `capture_version` | int | `1` when the row carries the facts below. A row without it was captured earlier: its zeros mean "not captured", not "none". |
| `start_context` | JSON string | What the session was given at start; see below. |
| `start_context_bytes` | int | `instruction_bytes + skill_bytes` of `start_context`. |
| `tool_results` | int | Tool calls that got a result. |
| `tool_errors` | int | Tool calls whose result was an error. |
| `tools_before_edit` | int | Tool calls before the first edit or commit; `-1` when the session made none. An edit is an Edit/Write/MultiEdit/NotebookEdit/apply_patch-style tool call, or a shell command that executes `apply_patch` or `git commit` (code-mode `tools.apply_patch(…)` included). |
| `command_counts` | JSON string | Kit commands run through shell tools: `{"board_search": n, "board_post": n, "traces_search": n, "dispatch": n}` for `hev board search`, `hev board post`/`reply`, `hev traces search` and any `hev dispatch` call. Only non-zero keys appear. |

Command facts count executed commands only. The tool's script is parsed as shell (Codex code-mode JavaScript first yields its `tools.exec_command` scripts: a literal `cmd`, a template literal with its `${…}` parts as parameters, and, when `cmd` is a name, the literals that name's nearest binding before the call gives it: `const c = "…"`, a `for (const c of list)` or `list.map(c => …)` / `list.map(async ([name, c], i) => …)` over a literal list (of strings, of `{cmd: …}` objects, or of tuples it destructures at any depth), or `const o = {cmd: "…"}` passed as `tools.exec_command(o)`. Any other value — computed, a function parameter, built with `+` — counts nothing, and strings and objects the call does not use are data however they read. Each call site counts each distinct command it resolves to once, so a template literal inside a loop counts once), and each simple command is matched on its words: launch wrappers (`env`, `sudo`, `timeout`, `with-op`, …) and hev's global flags are skipped, and `sh -c`/`bash -lc` scripts and command substitutions are followed. Words inside quoted arguments, heredoc bodies fed to another program, comments and JavaScript strings are data: `printf 'hev board search x'` or `grep "hev dispatch" docs/` counts nothing. A script that does not parse counts nothing rather than a guess, and a command run through a custom wrapper script (`./run.sh hev dispatch …`) is not recognized. Rows captured before capture version 1 are derived the same way from their block rows.
| `skill_loads` | JSON string | Each skill's first load: `[{"name","via","turn_uuid","seq","ts"}]`, at most 64. The block rows keep every load. |

`start_context`:

```json
{
  "source": "transcript",
  "instructions": [
    {"path": "~/.claude/CLAUDE.md", "kind": "User", "bytes": 3676, "sha256": "3f2a9c1b04de", "source": "transcript"},
    {"path": "CLAUDE.md", "kind": "Project", "bytes": 912, "sha256": "a01b2c3d4e5f", "source": "transcript"}
  ],
  "instruction_bytes": 4588,
  "skills": 21,
  "skill_bytes": 10694,
  "skills_source": "transcript",
  "context_digest": "sha256:…"
}
```

- `instructions` are the instruction files, at most 64: `path` relative to
  the session's repository (or `~/…` under home), `bytes`, and the first 12
  hex digits of the file's SHA-256. Never the content.
- `skills` and `skill_bytes` are how many skill descriptions the session was
  offered and their total size.
- `context_digest` is the dispatch context bundle the worker was given, when
  there is one (the same value as the row's `context_digest`).
- `source` says where each part came from:
  - `transcript`: the harness recorded it. Claude Code records the CLAUDE.md
    files it loaded (an `instructions` attachment) and the skill catalogue (a
    `skill_listing` attachment). Codex records the skill catalogue
    (`<skills_instructions>`) and the AGENTS.md text it loaded, merged into one
    block, recorded as `DIR/AGENTS.md` with `kind: "merged"`.
  - `derived`: the transcript did not show it, so capture read it from disk at
    capture time: the harness's home instruction file, then each directory's
    instruction file from the repository root down to the working directory
    (Claude Code `CLAUDE.md`, `.claude/CLAUDE.md`, `CLAUDE.local.md`; Codex
    and OpenCode `AGENTS.md`; PI `AGENTS.md` and `CLAUDE.md`), and the skills
    under the harness's skill roots (name plus description bytes). This is
    today's disk, not necessarily what the session saw. PI transcripts do not
    record the starting context, so PI is always `derived`.
  - `mixed`: some of each; `none`: nothing was found.

### Block rows (`<namespace>-blocks`)

A `tool_result` row, and the `tool_use` row it answers, carry the outcome:

| Attribute | Type | Meaning |
|---|---|---|
| `ok` | bool | (existing) `false` when the result was an error. |
| `resulted` | bool | A result was seen; `ok` is only meaningful when it is `true`. |
| `exit_code` | int | The exit status the harness reported, read only from the harness's own metadata around a shell tool's output, never from the output itself; absent when none was, and always absent for non-shell tools (a `read_file` whose document says `Exit code: 2` has none). Claude Code `Bash`: `0` for a successful call, `N` from a failed one's leading `Exit code N`. Codex code mode (`exec`, `wait`): the `exit_code` of each exec envelope the script emitted, an object carrying `wall_time_seconds` or `chunk_id` beside it, decoded as JSON whatever its key order or spacing, inside orchestration wrappers (`{"status":"fulfilled","value":…}`, `{"session":…,"result":…}`, arrays) and when truncated; the first non-zero one when a script ran several commands. Codex direct shell tools (`exec_command`, `write_stdin`, `shell`, …): the `Process exited with code N` or `Exit code: N` header line before `Output:`, or `metadata.exit_code` of the JSON shell form. PI `bash`: its trailing `Command exited with code N` line. |
| `result_bytes` | int | The result's size as captured. Recorded whether or not the `tool_result` text tier is indexed for search. |

Codex records no error flag of its own, so capture marks a Codex result an
error when its script failed (`Script failed`) or its command exited
non-zero.

`skill_loads` events on block rows gain `via`: `tool`, `read` or `command`
(see [skill-capture.md](skill-capture.md), which also describes name
normalization).

### `hev traces show <id> --json`

`--json` now reads the session's block rows, so each turn has its `usage`
(`input_tokens`, `output_tokens`, `cache_read_tokens`,
`cache_creation_tokens`), `model` and `request_id`, tool results are present,
and every `tool_use` and `tool_result` block has `is_error`, `exit_code` and
`result_bytes`. For rows captured before tool outcomes, the outcome is read
from the stored result text. Without block rows it falls back to the search
chunks, as before.

## Run kinds

A session's run kind is derived from what its row already carries, never
typed by hand. The first rule that matches wins:

1. **`orchestrator-round`**: the dispatch `order` starts with `orch-`, or the
   working directory is under a dispatch root's `orch-*` directory
   (`~/dispatch/orch-NAME/rounds/NNN`).
2. **`worker`**: the row has a dispatch `order` or `trace_id`, or its working
   directory is under a dispatch root (`~/dispatch/NAME/…` on Linux, macOS or
   Windows homes).
3. **`loop`**: the session scheduled its own next run (it called
   `ScheduleWakeup` or `CronCreate`), was started by `/loop`, runs in an
   expediter's state directory, or its first prompt recurs: at least 3
   sessions outside dispatch in the window share the same working directory
   and whitespace-normalized first prompt (a scheduled task or a brief loop
   sends the same brief each time).
4. **`interactive`**: everything else.

## `hev traces stats`

```
hev traces stats [--since 5d] [--until TIME] [--by kind|harness|host|repo|order]
                 [--installed] [--scan-blocks=true] [--top 15] [--json]
```

- `--since` is the window back from now, or from `--until` (RFC3339, or a
  date meaning its end) for a fixed window.
- `--by` chooses the group dimension; `repo` is the remote's `owner/name`.
- `--installed` lists the skills installed on this host (every
  `NAME/SKILL.md` under `~/.claude/skills`, `~/.codex/skills`,
  `~/.agents/skills` and `~/.pi/agent/skills`) and those no session in the
  window loaded.
- `--scan-blocks` (on by default) gives rows captured before capture
  version 1 their tool facts by reading their block rows: tool outcomes are
  re-read from the stored result text and skill loads re-detected. It costs
  one block query per such session. The starting context cannot be derived
  this way; only the transcript or a backfill has it.

The text output has a line per group, the per-skill table (top 15; `--top 0`
for all), `--installed`'s list, and dispatch launch health by host.

### JSON (`hev.traces.stats/v1`)

`--json` prints one object. The shape is stable: `schema` changes only when a
field is renamed, removed or changes meaning; new fields may be added.

```json
{
  "schema": "hev.traces.stats/v1",
  "since": "5d",
  "from": 1794304800000,
  "to": 1794736800000,
  "by": "kind",
  "sessions": 878,
  "kinds": {"worker": 243, "orchestrator-round": 216, "loop": 341, "interactive": 78},
  "capture": {"sessions": 878, "with_capture_facts": 0, "derived_from_blocks": 878, "with_start_context": 0},
  "groups": [
    {
      "key": "worker",
      "sessions": 243,
      "tool_calls": {"median": 50, "p90": 121},
      "facts_sessions": 243,
      "edited_sessions": 180,
      "tools_before_edit_median": 23,
      "board_search_share": 0.1,
      "traces_search_share": 0.06,
      "board_post_share": 0.05,
      "dispatch_share": 0.25,
      "tool_results": 12000,
      "tool_errors": 700,
      "tool_error_rate": 0.058,
      "start_context_sessions": 0,
      "start_context_bytes_median": null,
      "tokens_in": 1470900000,
      "tokens_out": 7900000
    }
  ],
  "skills": [
    {"name": "peers", "sessions": 569,
     "by_kind": {"worker": 141, "orchestrator-round": 99, "loop": 325, "interactive": 4},
     "by_harness": {"claude_code": 4, "codex": 565},
     "by_via": {"read": 560, "tool": 9}}
  ],
  "installed": {"skills": ["…"], "never_loaded": ["…"]},
  "dispatch": {"runs": 180, "launch_failures": 12, "validation_runs": 30,
               "attempts": 250, "attempt_launch_failures": 17,
               "by_host": [{"host": "oven", "runs": 150, "launch_failures": 10,
                            "validation_runs": 25, "attempts": 205,
                            "attempt_launch_failures": 14}]}
}
```

| Field | Meaning |
|---|---|
| `from`, `to` | The window, Unix milliseconds. Sessions are selected by `start`. |
| `kinds` | Sessions per run kind. |
| `capture` | How complete the facts are: rows carrying them, rows that got them from block rows, rows with a recorded starting context. |
| `groups[]` | One per `--by` value, most sessions first. A key with no value is `(none)`. |
| `tool_calls` | Median and 90th percentile (nearest rank) of tool calls per session, over all sessions in the group. |
| `facts_sessions` | Sessions with tool facts; the shares, edit and error fields are over them. |
| `edited_sessions`, `tools_before_edit_median` | Sessions that edited or committed, and the median tool calls before the first edit or commit among them. |
| `*_share` | Share (0–1) of `facts_sessions` with at least one `hev board search`, `hev traces search`, `hev board post`/`reply`, or `hev dispatch` call. |
| `tool_results`, `tool_errors`, `tool_error_rate` | Summed results and errors, and their ratio. |
| `start_context_sessions`, `start_context_bytes_median` | Sessions with a recorded starting context, and its median size. |
| `tokens_in`, `tokens_out` | Summed input (input, cache read and cache creation) and output tokens. |
| `skills[]` | Per skill, most sessions first: sessions that loaded it (normalized name), by run kind, harness and `via` (`by_via` only counts rows that recorded how). |
| `installed` | Only with `--installed`. |
| `dispatch` | Dispatch launch health over the runs started in the window (`attempt` rows of the spans namespace with a key; this host's run records without one). A run is one `run_key` (else its run directory on its publisher), however many attempts (reworks, relaunches) it had, and is counted on the host and in the window of its first attempt. The span query is windowed on attempt start, so the command then reads every attempt of each run with an attempt in the window (hosted: by the filterable `run` attribute, keeping rows whose run key matches); a run first launched before the window and reworked inside it is therefore not a run started in the window. `runs` and `launch_failures` cover work runs; a run is a launch failure when any of its attempts ended failed (non-zero exit, or an unconfirmed launch) or blocked within two minutes of that attempt's launch. `validation_runs` counts runs dispatched to validate another run (their attempts carry the parent run as `source`), kept out of `runs`. `attempts` and `attempt_launch_failures` count every attempt of the window's runs, validation runs included. |

A field that needs data no session in the group had is `null`.

## Backfill

New sessions are captured with the facts. To add them to the last 30 days of
sessions, run on each host that holds transcripts:

```
hev traces backfill                 # preview: what the rows would carry, per harness
hev traces backfill --apply         # write them
```

The preview parses the transcripts touched in the window and reports, per
harness, units, sessions, block rows, skill loads by `via`, starting context
by source, and tool calls with a result, an exit code and an error. It writes
nothing.

`--apply` rewrites only the read side (block and session rows, through the
same guarded writers `hev index --read-side` uses, with the redaction pass);
it writes no search chunks and embeds nothing. It journals each transcript as
it lands in `~/.hev/traces-backfill.json`, so an interrupted run resumes where
it stopped: run it again. The window is fixed when the journal is created
(`--since`, default `30d`, at most `90d`) and kept until `--restart`.
It is closed at both ends: a transcript (or an OpenCode session, by its last
recorded activity) is selected only if it was last touched inside the
window. Anything touched after the window's end, including transcripts that
arrive while a backfill is resumed over several runs, is never selected; the
live indexer captured it with the same capture when it changed, provided the
new `hev` was installed before the backfill's first `--apply`. The preview's
`OUTSIDE` column (`units_outside_window`) counts what the window leaves out.
`--limit N` writes at most N transcripts per run. Transcripts already aged out
of the host keep their old rows; `hev traces stats` derives their tool facts
from block rows.
