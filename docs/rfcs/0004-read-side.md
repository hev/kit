# RFC 0004: The read side — traces, timeline and stats on real data

Tracking issue: https://linear.app/hevmind/issue/LYR-26

**Status: draft. The design is mocked and reviewed (`mock/`, rendered from
1,643 real transcripts); no implementation started. Depends on RFC 0003's
namespace and adds two more.**

## Summary

`hev serve` puts a browser on the archive. What changes for the user: open
one page on the laptop and see every trace the fleet ran, the mini's
included — spend, tokens, cache hit and $ per prompt over a window; a
sortable, filterable list of traces with summaries and repo links; and for
any trace, the harness replayed read-only, a gantt that says where the
wall-clock went, and the trace's own stats. Nothing is rendered from files
on disk: the store is the only source. `hev serve` runs on the mini, which
is headless in the sense of having no screen, bound to its tailscale
address; the laptop's browser is the screen.

A Docker home lab (`deploy/homelab/`) stands up Layer CE, object storage and
kit together, so the same page runs against a stranger's traces in one
`docker compose up`.

## Motivation

- **The harness deletes history.** Every transcript on the laptop is under
  30 days old; Claude Code's default `cleanupPeriodDays` is 30. The archive
  is the only copy that survives, and today nothing reads it.
- **`hev find` returns a chunk; the questions are about sessions.** The
  mock's numbers answer them: one four-hour session was 83% waiting on the
  human, 16% model, 1% tools; 30 days on one machine cost $17k at list
  rates; cache hit is 98% overall but 8% on the costliest request. None of
  that is reachable from the chunk namespace.
- **The field has the same gaps.** Three surveys (`mock/NOTES.md`) found no
  tool that draws waiting-on-human as a lane, subagents as parallel lanes,
  commits on the same axis as tool calls, or context growth with compaction
  marked. The mock does all four from data the harness already writes.
- **kit needs to run somewhere other than a laptop.** RFC 0002 made every
  kit install a Layer install; this makes the mini's traces readable
  without anyone logging in to the mini.

## Decision

**Three namespaces, one page, one binary.**

1. **`<ns>` stays as RFC 0003 wrote it**: chunks, embedded, what search reads.
2. **`<ns>-blocks`**: one row per block, whole, never split, no vector column.
   A `tool_use` row carries `start`, `end`, `ms`, `tool_name`, `ok`; an
   assistant row carries `model`, `effort`, `request_id`, the four token
   counts and `cost`; every row carries `session_id`, `turn_uuid`, `seq`,
   `agent_id`, `parent_agent_id`. Blocks are the spans. The transcript and
   the gantt read this and nothing else, which is also why `tool_result`
   lands here whole while it stays opt-in for search.
3. **`<ns>-sessions`**: one row per session. `summary`, `first_prompt`,
   `harness`, `model`, `repo_url`, `branch`, `host`, `start`, `end`,
   `wall_ms`, `api_ms`, `idle_ms`, prompt, tool and request counts, tokens
   by type, `cost`, `has_subagents`. The Traces list and the Stats page read
   this; a 90-day window is a few thousand rows, aggregated in the binary
   until the store's `aggregate_by` is verified to cover it.

`hev index` writes all three from the same parse. Content-addressed ids
hold; RFC 0003's orphaning caveat applies to every namespace.

**`hev serve`** is a Go binary serving the mock's page and a JSON API over
the three namespaces: `/api/sessions?window=30d`, `/api/session/<id>`,
`/api/stats?window=30d`. No JS build; the page is vanilla, as mocked.
Binds `127.0.0.1` by default; `--bind` takes the tailscale address on the
mini, and the tailnet is the door. Auth is not v1.

**Summaries.** Where the harness wrote an `ai-title`, use it (400 of 1,643
had one). Otherwise `hev index --summarize` asks the harness the operator
already has (`claude -p --model haiku`) for one line and writes it to the
session row. kit still runs no model; it delegates to the one on the box.

**Repo identity** comes from `git remote get-url origin` in the transcript's
cwd at index time. Projects are keyed by repo URL, not cwd, so factory
worktrees fold into their repo.

## Where it runs

- **The mini, headless.** `com.hev.serve` runs `hev serve --bind
  <tailscale ip>` under launchd, restarted by the deploy job on every push
  to main. `hev daemon install` gains an index loop under the existing
  launchd job, so echo indexes its own `~/.claude/projects` into the
  shared namespace.
- **The laptop.** `hev index` for its own transcripts, and a browser
  pointed at the mini. The page shows both hosts, keyed by `host`.
- **The home lab.** `deploy/homelab/docker-compose.yml`: Layer CE, MinIO,
  and `hev serve`, with `hev index` run from the host against the compose
  endpoint. The embedding leg is the one CE decision this RFC does not make;
  see open questions.

## Milestones

1. Session and block rows written by `hev index`; `hev ls` reads session
   rows instead of the ledger.
2. `hev serve` with the three endpoints and the page ported from `mock/`.
3. `hev serve` under launchd on echo and the index loop beside it,
   both hosts indexed, summaries backfilled, the page open on the laptop.
4. `deploy/homelab/` compose file and a README that goes from clone to the
   page in one sitting.

## Goals

- The page answers the four questions the mock answers, from the store only.
- A trace that ran on either host opens in the laptop's browser, served
  from echo.
- A fresh operator runs the home lab and sees their own traces in a sitting.

## Non-goals

- Auth, multi-tenant, or a hosted UI.
- Playback. Scrubbing is a rail, not a player; markers over playback.
- Codex parity in the transcript view. Codex rows land; rendering follows.
- Retrieval changes. `hev find` stays as RFC 0003 left it.

## Vocabulary

- **Trace** — the operator-facing word for a session, the unit of the list.
- **Block** — a whole block as one row in `<ns>-blocks`; the span unit.
- **Window** — the range the Stats and Traces views are scoped to.
- **Summary** — the one line on a trace row; harness title first, backfill
  second.

## Open questions

- Does the store's `aggregate_by` cover the Stats page, or does the binary
  keep aggregating session rows in memory? Verify before milestone 2.
- Layer CE embedding in the home lab: hosted gateway for the vector leg, or
  FTS-only chunks until CE embeds locally? FTS-only is a one-way door
  (RFC 0003), so the compose file must pick one.
- Subagent time is drawn beside the main thread and left out of the
  wall-clock split. Is that the right accounting for a fleet of gaffers,
  where the subagents are the work?
- Should `<ns>-blocks` carry FTS on `text` so `hev find --raw` can grep
  tool results without embedding them?
- Rates. The mock hardcodes list prices; where do they live, and is cost
  computed at index time or at read time?
