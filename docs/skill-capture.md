# Skill capture and backfill preview

Normalized tool blocks carry `skill_loads` events (name, recorded SKILL.md
path, source root, turn UUID/sequence, timestamp, optional commit). Session
`skills` is a sorted unique list, explicitly filterable. Event JSON is not
filterable. Paths retain transcript spelling; absent paths and historical git
commits stay empty. Reading today's git HEAD cannot establish a past revision.

Claude Skill calls provide the input `skill` name; expanded slash commands are
recognized only with explicit “Base directory for this skill:” provenance.
Read tools and shell cat/head/sed/less/more reads of skills/**/SKILL.md are
recognized across Claude, Codex, PI and dot. Tool results and ordinary markdown
reads do not count. Events represent recorded load attempts, not proof that a
tool succeeded. Dynamic shell paths and opaque wrapper commands may be missed.

Preview existing local transcripts without credentials or network/store access:

```
go run ./cmd/hev skills-preview codex /path/to/rollout.jsonl
go run ./cmd/hev skills-preview claude_code /path/to/session.jsonl
go run ./cmd/hev skills-preview pi /path/to/session.jsonl
go run ./cmd/hev skills-preview dot /path/to/session.jsonl
```

Multiple files are accepted. Each output reports harness, sessions_scanned,
sessions_with_skills (sessions that would gain the new attribute), and candidate
session rows. Run separately per harness. No apply mode exists. A later
operator-authorized reindex of original transcripts can persist events and
rollups; unchanged-unit checkpoints must be bypassed by that reindex. This
worker did not access or modify any live namespace. Fixtures are synthetic,
shaped from repository transcript formats; production coverage counts require
running this preview where the original transcripts reside.

Dashboard trace lists and detail headers show skill names. The skill facet
selects sessions using any chosen skill (`?skill=review`, repeatable). Older
sessions have an empty list until reindexed.
