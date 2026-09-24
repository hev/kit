---
name: hev-query
description: Search past coding-agent sessions (Claude Code and Codex, every machine running hevd) with `hev query`, hybrid semantic + full-text search over the hev kit trace archive. Use when asked what was done, tried, decided or said in an earlier session; whether an error, bug or approach has come up before; how something was fixed last time; which session touched a file, command, PR or plan; or to recover context after a transcript was deleted or compacted. Prefer it over the harness's own session history, `--resume` pickers, or grepping ~/.claude/projects and ~/.codex/sessions.
---

# Search traces with hev query

`hev query` searches the hev kit trace archive: every Claude Code and Codex
session captured by `hevd`, on every machine that runs it, in one Layer
namespace. Each query runs a dense (embedding) leg and a BM25 leg in the store
and fuses them with reciprocal rank fusion. `find` and `q` are aliases.

Use it instead of the harness's built-in history:

- **It covers everything.** Built-in history sees one harness on one machine,
  and Claude Code deletes transcripts after 30 days. The archive holds both
  harnesses from every machine, and it keeps sessions after the local files
  are gone.
- **It matches meaning and exact words.** Grep finds only the exact string,
  and a resume picker finds only titles. The dense legs match paraphrases, and
  the BM25 legs match exact tokens like `ECONNREFUSED`, `--force-conflicts` or
  `LYR-84`.
- **It fans out in one request.** Up to eight phrasings run as legs of one
  query, and the store fuses them, so you read one ranked list.

## Preflight

Run `hev s`. If the daemon isn't running or `hev query` fails with a
connection or auth error, tell the user and suggest `hev up`. Don't run
`hev up` yourself: it starts Docker containers and installs a launchd agent.

## The loop: plan, fan out, gather, judge, read

This is the same loop Layer's agentic search runs server-side
(hevlayer.com/docs/api/agents): plan phrasings, fan out for recall, gather
and dedupe, judge relevance, then read. A single literal query is the weakest
way to use the archive.

### 1. Plan

Split the question into one to four sub-questions. For each one, write two to
five phrasings that differ in kind, not just wording:

- **Plain language:** "why did the turbopuffer preflight fail"
- **Literal tokens** you expect in the transcript: error strings, env vars,
  flags, file paths, command names, PR and ticket ids, e.g.
  `TURBOPUFFER_API_KEY 401 preflight`. These feed the BM25 legs.
- **The agent's voice:** most indexed text is assistant prose and tool calls.
  Write what the agent would have said: "Build failed. Let me check", "the
  root cause is", "fixed by", "reverting".
- **The user's voice:** how the request was probably phrased: "can you make
  the deploy stop timing out".

Tool results (command output, file contents) are **not** indexed by default.
A raw error message usually lives in a tool result, so search for the
assistant's reaction to the error and for the command that produced it.

### 2. Fan out

Put the phrasings of one sub-question in one call. The store fuses every leg:

```bash
hev query --json --top 15 "why did the preflight fail" \
  --also "TURBOPUFFER_API_KEY missing 401" \
  --also "preflight check failed, let me look" \
  --also "fix the preflight"
```

Fan out across separate calls, in parallel, when the calls need different
scopes: different sub-questions, time windows (`--since`), harnesses
(`--harness`) or repos. Make the calls as
parallel tool calls in one turn, or scatter them from one shell and gather
the results:

```bash
d=$(mktemp -d)
hev query --json --top 15 --since 14d "deploy timed out" --also "helm upgrade timeout" > "$d/recent.json" &
hev query --json --top 15 --harness codex "deploy timed out" --also "rollout status deadline exceeded" > "$d/codex.json" &
hev query --json --top 25 "deploy timeout workaround" \
  | jq --arg w "$PWD" 'map(select((.workdir // "") | startswith($w)))' > "$d/here.json" &
wait
```

Scope to a repo with the `jq` prefix filter above rather than `--workdir`,
which fails on namespaces whose `workdir` attribute isn't filterable. A
prefix covers the repo's subdirectories. Worktrees kept elsewhere, such as
`../.worktrees/<repo>/<branch>`, need their own prefix or a
`test("/<repo>(/|$)")` match.

### 3. Gather

Merge the results, dedupe by turn and rank sessions by how many legs found
them. A session that turns up across phrasings and scopes is the strongest
signal you get:

```bash
jq -s 'add | unique_by(.session_id + .turn_uuid + .text)
  | group_by(.session_id)
  | map({session: .[0].session_id, hits: length,
          first: (map(.ts) | min), workdir: .[0].workdir, harness: .[0].harness,
          turns: map({turn_uuid, ts, role, block_type, tool_name, text: .text[:300]})})
  | sort_by(-.hits)' "$d"/*.json
```

### 4. Judge

Hybrid search always returns rows: the dense legs return nearest neighbours
even when nothing is relevant. Read every hit and throw out the ones that
don't answer the question. Also discard:

- searches, not evidence: hits whose text runs `hev query` or `hev find`, or
  quotes your query back. These come from this session or another one
  searching the same thing, and they rank high because they contain your exact
  words. Be most suspicious of hits from today;
- encoded blobs, such as base64 or encrypted reasoning in Codex tool calls;
- exact duplicates: Codex can store the same message twice.

If nothing survives, go back to step 1 with new vocabulary. Take it from the
near misses: the file names, commands and terms that did appear.

### 5. Read around a hit

Don't print a whole trace. A session can run past 200 KB and thousands of
turns. Open a window around the hit, found by its `turn_uuid`, or by its `ts`
when older rows carry a different turn id:

```bash
hev trace --json <session_id> | jq --arg t <turn_uuid> --arg ts <ts> '
  ((map(.turn_uuid) | index($t)) // (map(.ts >= $ts) | index(true))) as $i
  | if $i == null then "hit not in trace: sessions over 10,000 chunks are cut off"
    else .[([$i-4,0]|max):$i+6]
      | map({seq, ts, role, blocks: (.blocks | map({type, text: .text[:1500]}))})
    end'
```

If the hit isn't in the trace, work from the hit text and from other hits in
the same session: rerun `hev query --json` with the session's vocabulary and
keep the rows whose `session_id` matches.

Widen the window or step forward from the hit to see how the problem was
resolved. `hev trace` takes a session-id prefix. To browse by time, run
`hev ls --since 3d` (add `--long` for full ids). Its titles are session
summaries.

### 6. Answer

Cite what you found: the date, harness, working directory and session id
prefix (`hev trace 2b00ee12` works), plus a short quote. Say what you
searched for and didn't find. Traces hold whatever the agent saw, secrets
included, so don't repeat keys or credentials from a hit.

## Reference

`hev query <query> [--also <phrasing>]... [flags]`

| Flag | Meaning |
|------|---------|
| `--also` | Another phrasing, fused in the same request. Repeatable, eight phrasings in total. Needs a turbopuffer-backed namespace. |
| `--top N` | Rows returned after fusion (default 8). Use 15–25 when you'll gather and judge. |
| `--json` | Full chunk text plus `session_id`, `turn_uuid`, `ts` (UTC), `harness`, `role`, `block_type`, `tool_name`, `workdir`, `plan`, `pr`. Always use it when you're the reader. |
| `--since` | Only chunks newer than a window: `90m`, `36h`, `7d`. |
| `--harness` | `claude_code` or `codex`. |
| `--workdir` | Exact match on the working directory. Worktrees have their own paths, so `…/.worktrees/kit/foo` doesn't match `…/kit`. On turbopuffer, `workdir` is not yet filterable, so this errors there; use the `jq` prefix filter instead. |
| `--plan` | Only chunks tagged with this plan. |
| `--namespace` | Another Layer namespace (default from `~/.hev/config.toml`). |

Every scoping flag is ANDed with the others. `role` is `user` or `assistant`.
`block_type` is `text` or `tool_use` (`tool_result` only if the user ran
`hev index --tier all`).
