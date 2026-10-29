# Skill capture and backfill

The normalized model records `skill_loads` on the tool block that loaded a skill.
Each event contains `name`, `path`, `source` (the path through `skills/`), `turn`,
`seq`, `time`, and `commit`. Unknown optional values are omitted. The session
`skills` attribute is a sorted, unique list of names and explicitly declares
`filterable: true`. The block event JSON is stored as a string with
`filterable: false`; the API decodes it back into events.

Claude Code's `Skill` input supplies the name. Recorded slash commands with
matching command-name and command-message markers also supply a name. A native
invocation that records no filesystem location leaves path and source empty.
Read/ReadFile tools and shell reads (`cat`, `sed`, `head`, `tail`, `less`, `more`,
`bat`, `read_file`) recognize paths ending in `skills/<name>/SKILL.md`, including
nested roots such as `.codex/skills/.system/<name>`. Codex exec wrappers, PI read
and bash calls, Claude Read/Bash calls, and dot read/exec calls share detection.
Repeated loads remain separate events; multiple mentions in one call are
collapsed. Ordinary markdown and tool results are not treated as loads.
Detection describes recorded load calls; it does not prove that the caller used
the instructions afterwards. Dynamic shell variables/globs or indirect scripts
that hide the filename cannot establish a load. No current git checkout is
consulted: these transcript formats do not record the skills repository's commit,
so historical commit is left empty rather than inferred from session/repo git data.

The dashboard shows names beneath each trace title and in the trace header.
The expandable skill loads section shows recorded paths and load times.
The `skill` picker and `?skill=NAME` filter use the session `skills` list.
Repeated skill query parameters match any selected skill. Names shared by roots
share a filter value; event paths and sources preserve their origin in trace API
`skill_loads`.

## Preview without a namespace

Build/run from the clone. Supply only local transcript roots you want scanned:

```sh
go run ./cmd/hev skills-preview \
  --claude_code /snapshot/.claude/projects \
  --codex /snapshot/.codex/sessions \
  --pi /snapshot/.pi/agent/sessions \
  --dot /snapshot/.dot/traces > skills-preview.json
```

This command has no store client, credential resolution, schema operations, or
apply mode. It reports sessions scanned and `would_gain_skills` per harness,
with candidate session IDs and skills. Counts represent sessions with detected loads
that would gain skills when upgrading the pre-skill archive; they are not a
comparison against a live namespace. Only sessions whose transcripts still
exist in the supplied roots can be recovered. Parse errors fail the preview.
The command reads all selected transcripts into memory to aggregate sessions
across files; for large archives, scan bounded snapshot directories.

A reproducible small preview using the checked-in harness-format fixtures:

```sh
go run ./cmd/hev skills-preview \
 --claude_code internal/trace/testdata/skills/claude_code.jsonl \
 --codex internal/trace/testdata/skills/codex.jsonl \
 --pi internal/trace/testdata/skills/pi.jsonl \
 --dot internal/trace/testdata/skills/dot.jsonl
```

A root can be a directory or one JSONL file. Keep each harness's input separate.
The preview and fixture tests expect one session gaining skills for each of the
four harnesses in this example.
Fixtures contain synthetic, secret-free calls adapted from the repository's
existing transcript formats; no private historical transcripts were collected.

## Applying later

The existing read-side reindex is the backfill write path:
`hev index --force --read-side --namespace TARGET`. It reparses available local
transcripts, writes whole block events and advances session source metadata,
without embedding chunks. Its configured/default source roots must match the
preview snapshot; `--root` overrides Claude's root, PI uses the daemon's
`pi_sessions_dir`, and Codex/dot use their normal homes. Run the preview first
and review counts and roots. Applying this command is a separate operator action;
it was **not run** for this candidate. No live namespace/schema was modified.
