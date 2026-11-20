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
hev dispatch close example-order 'PR merged by hand'
hev dispatch order-b.md --supersedes oven/example-order
hev steer example-order 'Scope changed: use candidate B' --level urgent
hev steer ls example-order
```

Workers read the full ../order.md, check ../inbox.md between steps, keep ../status.md current, and write ../report.md with acceptance evidence and named failures. Workers also write ../review-notes.json beside report.md, hunk's agent-context format with kit's `head`: a short changeset summary, one summary per changed file, and annotations only where rationale helps a reviewer, with `author` = run name and model and `source` = run name. A review run writes each finding as an annotation tagged `["finding","fail"]` or `["finding","note"]` with `confidence`, sets `head` to the SHA it reviewed, and keeps the `VERDICT:` first line of report.md. Missing notes never fail a run. Review a run with `hev review NAME` and return your hunk notes with `hev review send NAME` (docs/review.md). Follow explicit order instructions about commits and publishing. Never rerun a failed test just to obtain green.

Workers end by making the last line of ../status.md exactly one marker: `done`, `blocked: <reason>`, or `needs-decision <KEY>: <question>`. Ask for a decision only when the order leaves a real choice open, and stop after writing it. A run that needs a decision shows as `needs-review`, and `ls` shows its key and question in the DECISION column. Answer it with `say NAME --resolve KEY ANSWER`, which records the answer and reworks the same branch and PR. Use plain `say` for other notes. A blocked run shows as `blocked` with its reason.

When all the work you are authorized to do is complete and only a human step remains (a merge, a `sudo` command, a deploy), end with `done`, not `blocked:`, and name the handoff in report.md. Use `blocked:` only when something stops you from finishing your own part.

Close the loop on runs whose work finished another way. `hev dispatch close NAME [REASON...]` (or `--reason`, `--superseded-by [HOST/]OTHER`, `--host`, `--json`) marks an ended run `closed`: a terminal state that `ls` hides by default, follow-through treats as settled, and the dashboard ranks with `done`. It settles an open decision without a rework, keeps the log, reports, worktree and the worker's own reason, and refuses while the run is running. A later `rework`, `retry` or `amend` starts a fresh attempt and reopens it. When a new order replaces an old run (a `-b` order, another host), dispatch it with `--supersedes [HOST/]OLD_NAME` (or `supersedes:` in the frontmatter): the new run joins the old run's trace unless `--trace` is set, and once it is admitted the old run is closed with `superseded_by` pointing at it. If that close fails, dispatch warns and the new run still goes ahead; close the old one by hand.

Before you pick up a repository's work yourself or dispatch a fresh order for it, check `hev dispatch ls` for that repository's open `blocked` and `needs-review` runs. Prefer `rework`, `amend` or `say --resolve` on the existing run. If you do take the work another way, `close` the old run, or dispatch with `--supersedes`, in the same turn, so it stops surfacing as needing you.

`say` is a note: the worker sees it at its next turn, which for a long headless run may be never. When facts change under a busy worker, use `hev steer NAME MESSAGE --level urgent`: it lands at the worker's next tool call and blocks its file writes, git commit/push, `hev dispatch` and `gh pr` writes until the worker acknowledges. `--level halt` ends its turn (the run shows `halted`); `--level redirect` halts, commits a checkpoint and reworks the same run with your message. `hev steer` prints the delivery the run gets; `ls` shows receipts as `q1 i0 a2` (queued, injected, acked). Reading inbox.md is not an acknowledgement.

When you are the worker and kit injects a steering message, read it, act on it, then run the exact ack command it gives (`hev steer ack ID`); mutating tool calls stay blocked until you do. Do not ack what you have not acted on. On a halt, call no more tools and end your turn with one line saying where you stopped.
