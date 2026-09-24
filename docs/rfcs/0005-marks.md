# RFC 0005: Trace evaluations in Layer

Status: implemented. Extends RFC 0004 with a generic evaluation namespace.

`hev eval put [file]` (or `--file file`) reads newline-delimited JSON objects
from a file or stdin. The default namespace is the configured trace namespace
plus `-evals`. For example:

```json
{"session":"trace-uuid","ts":"2026-09-07T12:00:00Z","role":"reviewer","instance":"nightly","host":"laptop","marks":{"accuracy":4,"clarity":5,"coverage":5,"evidence":5},"poor":false,"summary":"The change passed review","evidence":{"accuracy":"Turn 2 verified the behavior","coverage":"assistant-turn-uuid checked the edge case"},"findings":["Add an example of the empty response"]}
```

The names above are examples, not reserved values. kit declares no role,
instance, or mark vocabulary. Marks are integers; their scales and the meaning
of `poor` belong to the evaluator. Mark names begin with an ASCII letter and
contain letters, digits, or underscores. `session`, RFC3339 `ts`, and the
`marks` object are required. Evidence is a map of names to prose; findings is
an array of strings. Empty evidence and findings normalize to `{}` and `[]`.

The write schema is:

| Attribute | Store type | Meaning |
| --- | --- | --- |
| `id` | string key | First 32 hex characters of SHA-256(session + NUL + canonical UTC ts) |
| `text` | string, FTS + embedding | Summary, evidence sorted by name, then findings, newline joined |
| `session_id` | string | Trace join key |
| `ts` | string | Canonical UTC RFC3339 timestamp |
| `role`, `instance`, `host` | string | Producer attribution, filterable |
| `poor` | bool | Evaluator's overall flag |
| `mark_<name>` | int | One dynamically declared filterable column per mark |
| `summary`, `marks`, `evidence`, `findings` | unfilterable string | Display summary and JSON-encoded original values |

`text` uses the configured chunk embedding model and cosine distance. Layer
embeds and performs hybrid retrieval; kit computes no vectors. Re-putting the
same session/timestamp addresses the same row, and an insert-only store
condition makes replay a no-op.
A changed grade should carry a new timestamp. Input is streamed in batches of
30; an error after a successful batch does not undo earlier batches, and the
file can safely be replayed.

The dashboard joins the newest timestamp per session. Eval filters (`poor`,
repeatable `role` / `instance`, and `mark_<name>_max`) run in Layer against
those newest row IDs, so an obsolete poor grade cannot select a now-good
trace. An absent eval namespace means ungraded traces; other store errors
remain visible. The same rule applies to search: only the newest evaluation
is searchable alongside transcript chunks. Results select each session's
best Layer RRF score across both sources; the server does not fuse rankings.
Search is bounded to candidate hits, so `top` is an upper bound on unique
traces, not a promise of that many distinct sessions.

The list displays marks in alphabetical name order, with names in the tooltip,
and a poor pill when set. The Eval panel displays evidence and findings. Prose
citing a loaded turn UUID or `Turn N` gains a link to that turn. Eval search
snippets open this panel; transcript snippets open their containing turn.

## Read-side filters and migration

Session rows now upsert by `session_id` and store `prompt_ts` (Unix
milliseconds as `[]uint`), distinct `tool_names` (`[]string`), and
`total_tokens` (all four usage counters). Existing content-addressed session
rows are folded by latest `end`; stable IDs win ties. Session upserts
conditionally accept only equal or newer `end` values, so replaying an older
file cannot roll back the stored state. Before mutable store filters run, the server resolves those latest row IDs, preventing old state
from satisfying a predicate. Session and eval scans paginate by ID through
the 10,000-row query cap. Latest-row filter reads use batches of 1,000 IDs.
Search uses one complete ID membership set per namespace so Layer ranks all
candidates together.

Repeatable project values accept display names or exact repo URLs; names
resolve to all matching stored repo URLs. Models, harnesses and hosts use
`In`, tools use `ContainsAny`; ranges use inclusive bounds. `since` includes
and `until` excludes session start timestamps. Date-only values mean UTC
midnight. Day charts use the same UTC boundaries; heatmap cells use browser
local time. The heatmap is the only archive filter evaluated in the browser.

After merge, the operator runs `hev index --read-side --force` once on each
capture host. No production backfill is performed by tests or this change.
The eval producer and its one-time backfill are a separate integration.
