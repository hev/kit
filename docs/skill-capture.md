# Skill capture and local backfill preview

Every parser attaches `skill_loads` to the relevant normalized block. Each event
contains the name, evidenced SKILL.md path, skills root, turn UUID, turn sequence
and timestamp. `commit` is reserved for historical version evidence and stays
empty when unavailable; capture never substitutes the current checkout's HEAD.
Claude's native Skill input and recorded `<command-name>/…</command-name>`
invocations are captured even when no file path is recorded. Unknown paths and
roots stay empty rather than assuming which of several skill roots won.

Codex, PI, dot and Claude read tools and shell reads (`cat`, `sed`, `head`,
`tail`, `less`, `more`, `bat`, `nl`, `awk`, `rg`, `grep`) of `skills/…/SKILL.md` count as loads. Nested
roots such as `.codex/skills/.system/imagegen/SKILL.md` retain `.codex/skills`
as their source. Prompt catalogues, tool results, ordinary markdown reads,
file listings, and SKILL.md.backup do not count. Shell reads are parsed with a shell syntax parser. Only literal file operands
of read commands count: comments, echo/printf arguments, grep patterns, sed/awk
programs and operands of separate commands do not. Quoted paths retain spaces.
Codex JavaScript orchestration supports literal `cmd`/`command` properties on
`tools.exec_command` calls through wrappers such as `text(await …)`, assignments,
and `Promise.allSettled`. A small tokenizer excludes JavaScript comments and
quoted examples before extracting calls; command strings still pass through the
shell operand parser. Dynamic command expressions are left unknown.
Shell detection is conservative;
dynamic variables naming a file, arbitrary Python/Node file IO, and aliases
cannot be reconstructed reliably. Events record invocation/read evidence,
not proof that the tool eventually succeeded or that the model used its contents.
No additional harness-native skill event was present in the checked-in native
fixtures. Fixtures in `internal/trace/testdata/skills` are synthetic, secret-free
examples using each parser's native transcript format.

Session rows contain a sorted distinct `skills` list, explicitly
`filterable:true` (native string array where supported, JSON string otherwise).
Block events use a JSON string attribute `skill_loads`, `filterable:false`.
The dashboard shows skill names beside each trace title, offers a skill facet,
and shows individual loads with paths, roots and turn/time in the trace overview.
Dashboard skill filtering also runs locally so pre-capture archives and stores
without array predicates work.

## Preview without a backend

```
go run ./cmd/hev skills-preview \
  --claude-root /path/to/claude/projects \
  --codex-root /path/to/codex/sessions \
  --pi-root /path/to/pi/agent/sessions \
  --dot-root /path/to/dot/traces \
  --out /tmp/skills-plan.json
```

All roots default to their usual local harness locations. The command only reads
local transcripts and writes the optional local output file (mode 0600; refuses
to overwrite). It has no backend client, credential lookup, schema change, or
apply mode. It prints JSON counts by harness: `sessions`, `with_skills`, and
`would_gain`. With no baseline, `would_gain` assumes previously empty skills,
which is appropriate for archives predating capture. Pass
`--existing-sessions /path/to/sessions.json` to compare against a local JSON
array of existing session rows. Counts concern distinct sessions in the selected
local corpus, not sessions whose transcripts have aged out or live elsewhere.
Invalid transcripts fail the preview rather than silently undercounting.

The optional plan contains `sessions` and `blocks` with events for review.
Do not upsert it blindly: session rows also contain unrelated metadata. After
review and separate authorization, the existing
`hev index --force --read-side --namespace <approved-target>` path reparses
local sources and uses the normal guarded read-side writer; this also updates
blocks without embedding. A write installs the new schema attributes on that
target. Preview itself installs nothing. No live backfill was run for this order.

## Rework regression evidence

The `*-boundaries.jsonl` fixtures cover shell comments, paths with spaces,
README reads followed by echo (with `&&` and `;`), pipes, quoted echo payloads,
search patterns, ordinary markdown and each supported read command across all
four native transcript formats. `claude-native.jsonl` contains only native
Skill calls, including one with recorded path provenance; no Read call can mask
a missing native event. These remain synthetic fixtures: this isolated clone
does not contain an authorized real historical transcript corpus.

Run the entirely local coverage preview check with:

```
go test ./cmd/hev -run TestSkillsPreviewIsLocalAndCountsHarnesses -v
```

It builds isolated roots for all four harnesses, reports one session gaining
skills per harness, then confirms that replay against the exported baseline
reports zero gains. No backend access is needed.

The permanent validator probes and JavaScript wrapper regressions run with:

```
go test ./internal/trace -run 'TestIndependentSkillProbe|TestSkillJavaScriptWrappers' -v
```
