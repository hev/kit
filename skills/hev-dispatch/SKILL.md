---
name: hev-dispatch
description: Write and follow dispatch orders. Use when routing implementation work, inspecting an order, or working as a dispatched worker with acceptance requirements.
metadata:
  audience: owner
---

Write an order with YAML frontmatter naming name, host, repo, base, harness, model, validation and reply. Add concrete Acceptance items. Use configured host names; never guess routing. Add `context: PATH` to name a context recipe in the repository; dispatch builds its bundle at launch and gives the worker the assembled context on standard input (`hev context --help`, `hev context review --run NAME`).

When you dispatch for a person and the order's repository has instruction files (AGENTS.md, CLAUDE.md, `.claude/agents`, `.claude/skills`, `.agents/skills`), first do the context review in the `hev-context` skill: plan, propose a narrowed recipe, show the person one screen, then `hev dispatch order.md --context-file narrowed.json` and tell them the digest. If `hev context plan` finds no instruction files (kit has none), dispatch as usual without a recipe. If the person says to skip the review, dispatch without `--context-file`, and the worker loads the files whole. `hev dispatch` itself never prompts; a person dispatching by hand is not asked anything.

To give one run skills from the host's skills shelf, add `skills: [pack/skill, ...]` to the order (or `"skills"` to its context recipe). Only those skills are linked into that worker's harness for the run; at most one may come from a `mode` pack. `hev skills shelf` on the host lists what is pinned (docs/skills-shelf.md).

```sh
hev dispatch --help
hev dispatch hosts
hev dispatch order.md
hev dispatch show example-order
hev dispatch ls
hev dispatch say example-order --resolve db 'SQLite'
hev steer example-order 'Scope changed: use candidate B' --level urgent
hev steer ls example-order
```

Workers read the full ../order.md, check ../inbox.md between steps, keep ../status.md current, and write ../report.md with acceptance evidence and named failures. Workers also write ../review-notes.json beside report.md, hunk's agent-context format with kit's `head`: a short changeset summary, one summary per changed file, and annotations only where rationale helps a reviewer, with `author` = run name and model and `source` = run name. A review run writes each finding as an annotation tagged `["finding","fail"]` or `["finding","note"]` with `confidence`, sets `head` to the SHA it reviewed, and keeps the `VERDICT:` first line of report.md. Missing notes never fail a run. Review a run with `hev review NAME` and return your hunk notes with `hev review send NAME` (docs/review.md). Agents and orchestrator rounds review without hunk: start from `hev review NAME --json` (the worker's notes merged with earlier reviews' findings for the exact head, fetched over dispatch, never `gh`, so Azure Repos runs work too), record every review with `hev review record NAME --head SHA --file FINDINGS.json` (or `--clean`), and send findings with `hev review send NAME --recorded`, never as a free-text `say` (a running run gets them in its inbox and keeps running; an ended run is reworked with them on the same branch and PR); an orchestrator needs only `rework` for record and send. A head must be the run's branch head or an earlier head of it; one review holds at most 200 findings and 1 MiB. `hev dispatch show NAME` lists recorded reviews per head. Follow explicit order instructions about commits and publishing. Never rerun a failed test just to obtain green.

Workers end by making the last line of ../status.md exactly one marker: `done`, `blocked: <reason>`, or `needs-decision <KEY>: <question>`. Ask for a decision only when the order leaves a real choice open, and stop after writing it. A run that needs a decision shows as `needs-review`, and `ls` shows its key and question in the DECISION column. Answer it with `say NAME --resolve KEY ANSWER`, which records the answer and reworks the same branch and PR. Use plain `say` for other notes. A blocked run shows as `blocked` with its reason.

`say` is a note: the worker sees it at its next turn, which for a long headless run may be never. When facts change under a busy worker, use `hev steer NAME MESSAGE --level urgent`: it lands at the worker's next tool call and blocks its file writes, git commit/push, `hev dispatch` and `gh pr` writes until the worker acknowledges. `--level halt` ends its turn (the run shows `halted`); `--level redirect` halts, commits a checkpoint and reworks the same run with your message. `hev steer` prints the delivery the run gets; `ls` shows receipts as `q1 i0 a2` (queued, injected, acked). Reading inbox.md is not an acknowledgement.

When you are the worker and kit injects a steering message, read it, act on it, then run the exact ack command it gives (`hev steer ack ID`); mutating tool calls stay blocked until you do. Do not ack what you have not acted on. On a halt, call no more tools and end your turn with one line saying where you stopped.
