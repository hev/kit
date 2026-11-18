# RFC 0021: Scorers, benchmarks and self-improvement, on the operator's hosts

Status: Proposed (2026-11-18). Builds on [RFC 0005](0005-marks.md) (marks),
[RFC 0008](0008-orchestrators.md) (orchestrators),
[session outcomes](../session-outcomes.md), [label review](../label-review.md)
and [cost attribution](../cost-attribution.md). Aligns with RFC 0019 (gates,
PR #268) and the cluster trace review (PR #270). Amends RFC 0005 (see
"Amendment to RFC 0005"). Adapted from Warp Factories; every claim about Warp
cites a source in "Sources", and where Warp's docs are silent this RFC says so.

> Numbering: 0019 (gates) and 0020 (headless back of house, PR #278) are taken
> on open branches, so this is 0021.

## Summary

Kit records what agents did (traces), what happened to their work (outcomes:
PR, merge, CI, revert) and what it cost. It does not yet say whether the work
was *good*, and it has no way to compare one harness, model or instruction set
against another on the same work. Warp Factories ships three features for
this: scorers, benchmarks and self-improvement [W1]. This RFC adopts their
shape on kit's terms:

- A **scorer** is a named question about a finished run, answered with one of
  a few declared **labels**, each worth a score from 0 to 1. Its answer is
  written as an RFC 0005 eval. A scorer is either a **rule**, computed in code
  from outcomes and dispatch records, or a **judge**, a separate harness run
  with a rubric. Kit ships rule scorers first, because it already has the
  ground truth (merge, CI, revert) that an LLM judge can only guess at.
- Scorers are **host policy**. The host enables them in its TOML, which
  becomes hevd-owned under RFC 0019.
  A repository may *propose* scorers, but a host only runs a repository scorer
  it has pinned by digest. A run can never grade itself with a rubric it
  wrote.
- Scorers **observe and never block**. A gate (RFC 0019) can read a score or
  reuse a scorer's rubric, but it is then a gate, declared as one.
- A **benchmark task is a dispatch order** with a pinned base commit. `hev
  bench` builds tasks from past runs, runs configurations × repetitions through
  dispatch, scores every trial, and prints a comparison table.
- **Self-improvement is the improve orchestrator from RFC 0019** with a new
  source: scorer failures past a threshold become a dispatch order whose PR
  carries a "Regressions addressed" section. A person always merges.
- Judges run on the operator's harness CLIs and subscriptions. No hosted
  account is required.

## Motivation

- **Kit can count work but not judge it.** The dashboard can filter on
  marks, but kit ships no producer for them: RFC 0005 says "the eval producer
  and its one-time backfill are a separate integration". The evals that exist
  come from the factory repository's own scripts and a replayed historical
  corpus ([eval corpus replay](../runbooks/eval-corpus-replay.md)). Reviews of
  dispatch runs are bespoke reviewer orders, and their verdicts live in run
  directories, not in the archive.
- **Kit already holds better evidence than an LLM judge.** The outcomes sweep
  records whether the PR merged, which CI workflows passed at the final head,
  and whether the merge was reverted within 14 days. Warp's docs describe
  scorers only as LLM judges [W2], and its merge and autonomy figures as
  dashboard metrics that need a connected code host [W1]. Kit can make
  those signals scorers directly.
- **The cluster trace review (#270) shows the failure modes are
  observable.** 23 of 26 first reviews failed; five runs reported done with CI
  red or a check not run; 29 reworks were scope changes after the owner saw
  the work. Each of those is a question a scorer can answer for every run, not
  only the ones someone reviewed.
- **Model and harness choices are made by feel.** Adam picks `codex` or
  `claude`, and a model, per order. There is no way to replay ten real orders
  against two configurations and compare acceptance, cost and time.
- **The improve orchestrator needs an input.** RFC 0019 proposes an improve
  orchestrator that reads failed verdicts. Gate verdicts cover only what
  blocked a run. Scores cover what merged and still went wrong.

## Decision

1. **Scorers are files the host enables.** A scorer is
   `scorers/<name>/scorer.md`, with YAML frontmatter and the rubric as its
   body, the same layout as Warp [W5]. Host scorers live in
   `~/.config/hev/scorers/`. The host enables scorers in its TOML
   `[scorers]` table. A repository's `.hev/scorers/` holds proposals that a
   host adopts by name and content digest (Q1).
2. **Two kinds: `rule` and `judge`.** Rule scorers are built into kit and
   read outcomes, dispatch records and cost. Judge scorers run a separate
   harness session over a bounded evidence packet (Q2, Q8).
3. **One eval row per (session, scorer).** Results land in the RFC 0005
   `-evals` namespace with `role = "scorer"` and `instance = <scorer name>`.
   The score becomes an integer mark from 0 to 100, and the label is
   recorded alongside it. RFC 0005 is amended so the newest row per (session,
   role, instance) wins, not the newest per session (Q3).
4. **Scoring is fail-open and never blocks.** It runs when a dispatch run
   finishes, after capture for sampled sessions, when outcomes change, and on
   demand with `hev score`. A judge error leaves no score; it does not fail
   the run (Q4, Q5).
5. **Benchmarks reuse orders.** A task is an order with a pinned base SHA,
   kept Acceptance and an optional reference PR. A suite lists tasks,
   configurations, repetitions and scorers. Trials are dispatch runs that do
   not push (Q6).
6. **Self-improvement is an orchestrator source.** Failures past a threshold
   or age become an improve order. The PR it opens targets code, skills,
   instructions or a scorer, never host config, and a person merges it (Q7).

## Goals

- Every finished dispatch run on an enabled host gets rule scores, with no
  model call and no configuration beyond `hev up`.
- One judge scorer, `acceptance-met`, that can be calibrated against human
  labels in the existing `/review` page before anyone relies on it.
- `hev score report` answers "how often do runs on this repo meet acceptance,
  merge and stay merged, and at what cost", by harness and model.
- A benchmark can be built from real past orders in one command and run on
  the operator's own hosts.
- Repeated failures turn into reviewed PRs through the existing orchestrator
  and dispatch machinery.

## Non-goals

- **Blocking.** Scorers never hold a run, a merge or a release. That is
  RFC 0019's job.
- **A hosted control plane.** No scoring service, no account and no data
  leaving the operator's hosts except to the model provider of a harness the
  operator already uses.
- **A single quality number.** Kit reports each scorer separately, with
  counts, and never blends them into one overall grade.
- **Grading interactive sessions by default.** Phase 1 scores dispatch runs.
  Scoring a person's own sessions is opt-in per host (Q4).
- **Board retrieval evaluation.** `hev board eval` (PR #245) measures board
  recall and is unrelated.

## Vocabulary

- **Scorer:** a named question about a finished run, with declared labels.
- **Label:** one allowed answer, with a `score` from 0 to 1. Warp calls these
  classifications [W2].
- **Rule scorer:** computed by kit from records; no model call.
- **Judge scorer:** answered by a separate harness session reading an
  evidence packet and the rubric.
- **Evidence packet:** the bounded, redacted bundle a judge reads (Q2).
- **Subject:** what is scored: a dispatch run attempt at its head SHA, or a
  captured session. Both resolve to a trace `session_id`.
- **Pass:** a label whose score meets the scorer's `passing` threshold. A
  **failure** is a scored subject below it, as in Warp [W3].
- **Suite, task, configuration, trial:** as in Warp [W4]. A trial is one
  dispatch run of one task under one configuration.

## What Warp does and what kit adopts

Warp concepts are cited to the sources at the end. "Not adopted" items are
explained in "What kit does not copy".

### Scorers

| Warp | Source | Kit |
|---|---|---|
| `scorers/<name>/scorer.md`, frontmatter plus rubric body | [W5] | Same layout. Host directory `~/.config/hev/scorers/`; repository proposals in `.hev/scorers/` (Q1). |
| `name` (required; the directory is the slug) | [W5] | The directory name is the name. No separate field, so they cannot disagree. Warp's own example files omit `name` [W6]. |
| `description` | [W5] | `description`, required. It is shown in `hev score ls` and the dashboard. |
| `output: classification` | [W5] | Implicit. Kit scorers always classify. |
| `agents` (names an agent; `includeDescendants` adds child runs as evidence) | [W5] | `applies`: a selector over dispatch runs (`repos`, `classes`, `harnesses`) and optionally captured sessions. Kit has no named agents. Child runs (validators, reworks) are always in the packet as evidence. |
| `labels` (1–20; unique `value`, `score` 0–1, optional `description`; at least one passing, one failing) | [W5] | Same rules. `description` is required, because it is the judge's definition. Kit adds a reserved `unscorable` label (Q2). |
| `passingScore` (0–1) | [W5] | `passing`, 0–1. |
| `samplingRate` (0–100, default 10; 0 disables automatic scoring) | [W5] | `sample`, 0–100, default 100 for rule scorers and 10 for judges. Sampling is deterministic per (subject, scorer) (Q4). |
| `model` (required) | [W5] | `judge.harness` and `judge.model`, optional. The host's `[scorers]` defaults and overrides decide (Q4). |
| `runner`, `secrets`, `mcpServers` and `scorerDefaults` | [W5] | Not adopted. A judge runs with no tools and no secrets on a host the operator names. |
| `selfImprovement` (default false) | [W5] | `improve`, default false (Q7). |
| Automatic scoring of sampled runs on completion; records classification, score and reasoning | [W2] | Same, on dispatch finish and after capture. Kit records label, score, reasoning, evidence citations and findings (Q3, Q4). |
| On-demand scoring; rescoring replaces that scorer's previous result | [W2] | `hev score run`. The newest row per (session, scorer) wins, and older rows are kept for calibration (Q3). |
| Judge inputs | Not documented. The examples' rubrics refer to the pull request, the work item and the diff [W6], and the blog says a scorer reviews "how the conversation went and what it produced" [W7]. | Declared per scorer as `inputs`; assembled by code (Q2). |
| Judge input size limits | Not documented | Byte budgets per input and a total cap (Q2). |
| Built-in scorers | The docs name one, the "built-in Correctness Scorer" used by benchmarks [W4]. The blog describes Warp's own code-quality and efficiency scorers as examples [W7]. | Rule scorers from outcomes and records, plus three judges (Q8). |
| Judge calibration | Only "score a few runs manually and compare the judge's results against your own review" [W1] | Human labels from `/review` and `hev score calibrate` (Q2). |

### Benchmarks

| Warp | Source | Kit |
|---|---|---|
| `benchmarks/<suite>/suite.yaml` and `tasks/<task>.yaml` | [W5] | `benchmarks/<suite>/suite.toml` and `tasks/<task>.md`. A task is an order (Q6). |
| Suite `agent`: one agent per suite | [W4], [W5] | Not adopted. A suite's tasks are orders and may span repos. Configurations override harness and model. |
| `configurations`: 1–6, each varying model or harness per agent; the runner chosen at launch | [W4], [W5] | `[[config]]`: `harness`, `model`, `host`, and optionally `instructions` (a context recipe or skill set). No fixed limit; a cost estimate is printed before launch. |
| Task `title`, `prompt`, `successCriteria` | [W5] | Order `name`, body and `## Acceptance`. |
| `sourceRunId`; "turn it into a benchmark task" from a completed run | [W4], [W5] | `from:` and `hev bench task from-run RUN` (Q6). |
| `startingRepoRefs` pinned to full 40-character SHAs | [W5] | `base:` must be a full SHA in a task. |
| `tags`, `linearSeed`, `benchmarkRepoSubstitutions` | [W5] | `tags` adopted; `issue` exists in orders. Repo substitution is not adopted. |
| Repetitions chosen at launch | [W4] | `repetitions` in the suite, overridable with `--reps`. |
| Built-in Correctness Scorer judges `successCriteria` | [W4] | `acceptance-met` judges `## Acceptance`, plus any scorers the suite lists. |
| Blocked trials stay Blocked but can be scored | [W4] | Same. A `blocked` or `needs-decision` trial is scored on what it produced. |
| Results: overall recommendation "when at least two configurations have comparable results", efficiency and cost recommendations, total cost including scorer usage, run time, per-scorer averages and grids | [W4] | A CLI table per configuration with pass k/n, mean score, cost and time, and recommendations only when the evidence supports them (Q6). How Warp decides "comparable" is not documented. |
| Results frozen with the run | [W4] | `results.json` records the digests of tasks, scorers and suite (Q6). |

### Self-improvement

| Warp | Source | Kit |
|---|---|---|
| Opt-in per scorer | [W3], [W5] | `improve = true` per scorer, and host `[scorers.improve]` enables the source. |
| Trigger: 25 distinct unreviewed failures per agent, or the oldest unreviewed failure is 7 days old (defaults) | [W3] | `threshold` (default 10, range 1–50) or `max_age` (default 7d) per (scorer, repo). The blog instead describes a schedule "say every 24 hours" [W7]. |
| `failedRunThreshold` 1–50 | [W3], [W5] | `threshold`, 1–50. |
| What "unreviewed" means | Not documented | A failure is open until it is linked from an improvement PR or a person labels the judge wrong in `/review` (Q7). |
| "Run now"; cannot choose agents or failures | [W3] | `hev score improve --scorer X [--repo R]`, which can choose. |
| Follow-up run proposes a fix to app code or the factory definition | [W3] | An improve order whose PR targets code, AGENTS.md, skills, context recipes or the scorer. A host-config or gate change is only proposed, as a `hev host config` diff (Q7). |
| "Regressions addressed" section linking failing runs and scorer results | [W3] | Same section, generated, with run names, session IDs, labels and judge reasoning. |
| `reviewerType` (`admins`, `team`, `custom`, `none`) | [W3], [W5] | The requester of the improve orchestrator is requested as reviewer. There is no `none`. |
| "Nothing is adopted without your review" | [W3] | Orchestrators have no merge authority (RFC 0008, RFC 0019). |

## Proposed surface

### Q1. The scorer file, where it lives and who may change it

**Recommendation: host-owned, repository-proposed, pinned by digest.**

A scorer is a Markdown file with YAML frontmatter, the same as an
orchestrator spec (RFC 0008) and a Warp scorer [W5]:

```markdown
---
description: Did the run deliver every Acceptance item in its order, and only that?
kind: judge                      # judge | rule
applies:
  runs: dispatch                 # dispatch | session | both
  repos: [kit, layer]            # optional; default all admitted repos
  classes: [code, docs]          # optional order classes
inputs: [order, diff, report, outcomes, validation, transcript]
labels:
  - value: met
    description: Every Acceptance item is satisfied, and each one is shown by the diff, a test, or the run's evidence. Nothing outside the order's scope changed.
    score: 1
  - value: partial
    description: At least one Acceptance item is unmet or only asserted (claimed in report.md but not shown in the diff or evidence), or the run changed things the order did not ask for.
    score: 0.5
  - value: not_met
    description: Most Acceptance items are unmet, the diff does not address the order, or the run ended without a usable change.
    score: 0
passing: 1
sample: 100
judge:
  harness: codex                 # a request; the host's [scorers] table decides
  model: judge-model
budget:
  input_kb: 256
improve: true
---
You are scoring one finished dispatch run against the order it was given.

Judge the Acceptance items in the order, one at a time. For each item, cite
where it is satisfied: a file and line range in the diff, a test name in the
CI conclusions, or a line in the validation evidence. A claim in report.md
counts only if the diff or evidence shows it. Do not trust the PR body or the
report's description of itself.

Judge scope from the order text. Changes the order did not ask for (unrelated
refactors, version bumps, edited tests that hide a failure) make the run
`partial` even when every item is met.

If CI for the final head is red or was never collected, say so in your
reasoning, but judge the Acceptance items on the evidence you have.

If the packet is truncated where it matters, or the order has no Acceptance
section, answer `unscorable` and say what was missing.
```

Rules:

- The directory name is the scorer's name (`^[a-z][a-z0-9-]*$`). It must be
  unique on the host. Built-in scorers use names without a prefix; host and
  repository scorers may not reuse them.
- `labels`: 1–20, unique `value`s, `score` in [0, 1], at least one passing
  and one failing (Warp's rule [W5]). `unscorable` is reserved, and kit adds
  it to every judge. It writes no mark and is never a failure.
- Unknown keys are rejected, as in orders.

Where scorers live, and who may change them:

- **Host scorers** live in `~/.config/hev/scorers/<name>/scorer.md`. They are
  enabled in the host TOML:

  ```toml
  [scorers]
  enabled = ["pr-merged", "kept", "ci-green", "first-pass", "finished",
             "cost", "acceptance-met"]
  judge_harness = "codex"        # default judge; a scorer's judge.* is a request
  judge_model = "judge-model"
  judge_host = "oven"            # where judges run; default: the run's host
  judges_per_day = 200           # cap; past it, sampling stops and is recorded
  sessions = false               # score captured non-dispatch sessions (Q4)

  [scorers.adopt]
  task-compliance = "repo:kit@sha256:9f2c…"   # a repository proposal, pinned
  ```

- When RFC 0019's hevd-owned configuration lands, `[scorers]` and the scorer
  directory fall under it. A person with `config:scorers` changes them through
  `hev host config`, and the `config.write` tool class covers the scorer
  directory, so a run cannot edit the rubric that grades it. Until then they
  are ordinary files, as gates' rubrics are today.
- **Repository scorers** live in `.hev/scorers/<name>/scorer.md` at the
  repository's default branch. They are proposals. A host runs one only if
  `[scorers.adopt]` pins it by repository and SHA-256 digest of the file.
  When the file changes upstream, the pin stops matching. The scorer is then
  reported as `stale` in `hev score ls` and does not run until someone
  re-adopts it (`hev score adopt kit:task-compliance`). This is the
  admission principle applied to rubrics: the receiving host decides, and
  shipping a scorer in a repository grants it nothing.
- **Orders cannot declare or disable scorers.** An order may not opt out of
  scoring. A benchmark suite chooses scorers for its own trials (Q6).

Why not repository-only, as in Warp's factory-as-code, where merging the
definition applies it [W5]? In kit, the agents being scored commit to these
repositories. A worker that edits `.hev/scorers/acceptance-met/scorer.md` in
its own PR would grade itself with its own rubric the moment it merged.
Pinning by digest closes that, and keeps a scorer stable across a measurement
window, which a baseline needs.

### Q2. What the judge sees, and how it is bounded

**Recommendation: code assembles a bounded, redacted evidence packet from
declared inputs. The judge has no tools and reads only the packet.**

Warp's docs do not say what a judge receives or how large inputs are bounded
(see the scorer table). Kit makes it explicit, because what a judge can see
determines what it can fairly judge.

| Input | Source | Default budget |
|---|---|---|
| `order` | The run's `order.md`, including `## Acceptance` and the current rework instruction | 32 KB; never truncated silently |
| `diff` | `git diff base...head` in the run's worktree or from the PR. Always includes `--stat`. Lockfiles, vendored and generated files are listed but not inlined. | 96 KB |
| `report` | `report.md`, the final `status.md` line and `result.json` outcome | 16 KB |
| `outcomes` | The session's outcome fields: `pr_state`, `ci_conclusions` at the final head, `reverted`, `revert_state` | 4 KB |
| `validation` | RFC 0019 gate verdicts (`gates.json`) or today's `verdict.json`, with findings | 16 KB |
| `reviews` | PR review comments and reviewer-run reports for the head | 16 KB |
| `transcript` | The run's trace from the archive, reduced (below) | 64 KB |
| `cost` | Tokens, priced cost and wall time | 1 KB |

Transcript reduction is deterministic:

1. Keep every user message (the order and every rework or steering note).
2. Keep each assistant turn's text, capped at 1 KB per turn.
3. Reduce each tool call to one line: tool name, the tokenized command or
   path, and exit status. Keep the first and last 512 bytes of output only for
   calls that failed.
4. Keep the final assistant message whole.
5. If the result is still over budget, drop whole middle turns, oldest first.
   Put a marker where they were (`[turns 14–61 omitted: 212 tool calls, 9
   failed]`).

The packet begins with a manifest listing each input, its size, whether it
was truncated, and what was dropped. The rubric preamble tells the judge to
answer `unscorable` when truncation hides what it needs. The packet is built
from the trace archive, which is already redacted
([archive redaction](../archive-redaction.md)). The diff and report pass
through the same redactor before they reach the judge.

Judge output is strict JSON:

```json
{"label": "partial",
 "reasoning": "Items 1-3 are met (docs/x.md:10-40). Item 4 is only asserted in report.md; no test covers it.",
 "evidence": {"acceptance-1": "docs/x.md:10-40", "acceptance-4": "not shown"},
 "findings": ["Acceptance item 4 claimed in report.md without a test"]}
```

An undeclared label or invalid JSON is retried once. If it fails again, kit
records a scoring error in `hev score log` and writes no eval row.

**Calibration.** The label review page already shows machine predictions
and records human verdicts (`correct`, `incorrect`, `uncertain`) with history
([label review](../label-review.md)). Scorer evals become a `LabelSource`
kind per scorer. `hev score calibrate SCORER` reports agreement with human
verdicts, and a confusion table per label, separately for genuine samples and
probes (as label review already requires). A judge scorer is shown as
`uncalibrated` in reports until it has at least 30 human verdicts. `improve`
does not act on an uncalibrated judge.

### Q3. How results land in RFC 0005 marks

**Recommendation: one eval row per (session, scorer). The score becomes an
integer mark from 0 to 100, and the label is a string beside it. Amend RFC
0005 so each producer's latest row counts, not only the latest per session.**

A judge result for `acceptance-met` on run attempt `kit-help-20261110#2`:

```json
{"session": "0b9d…", "ts": "2026-11-18T23:02:11Z",
 "role": "scorer", "instance": "acceptance-met", "host": "oven",
 "marks": {"acceptance_met": 50},
 "labels": {"acceptance_met": "partial"},
 "poor": true,
 "summary": "acceptance-met: partial (0.5). Item 4 is only asserted in report.md; no test covers it.",
 "evidence": {"label": "partial", "reasoning": "…", "acceptance-4": "not shown",
              "subject": "oven/kit-help-20261110 attempt 2 @ 3e1c140",
              "scorer": "acceptance-met sha256:4b1e…", "judge": "codex judge-model",
              "packet": "diff truncated 96/140 KB"},
 "findings": ["Acceptance item 4 claimed in report.md without a test"]}
```

Mapping:

- **Mark name:** the scorer name with `-` replaced by `_`. This satisfies RFC
  0005's mark-name rule.
- **Mark value:** `round(score × 100)`. Integers keep RFC 0005's type, and
  100 steps preserve every score Warp-style labels use (0, 0.5, 1, thirds
  round to 33/67). `unscorable` writes no mark.
- **Label:** a new optional `labels` object, stored as a filterable string
  `label_<name>`. You can filter on "partial" without knowing it is 50.
- **`poor`:** `score < passing`. That is the scorer's failure, as defined in
  [W3].
- **`role` / `instance`:** `scorer` / the scorer name. Rule scorers and judge
  scorers write the same shape. Rule scorers set `evidence.rule` to the
  fields they read.
- **Subject:** a dispatch run attempt resolves to its harness session ID,
  which `result.json` already records. `evidence.subject` keeps the run name,
  attempt and head SHA, so a reader can find the run.

#### Amendment to RFC 0005

RFC 0005 joins **the newest eval per session**
(`internal/serve/evals.go: latestEvals`), and keys rows by SHA-256 of session
and timestamp. With several scorers on one session, the last scorer to write
hides the others, and two scorers that write in the same second collide.
Three changes:

1. **Latest per (session, role, instance).** Dashboard joins, filters and
   eval search use the newest row per producer. "Rescoring replaces the
   previous result from that scorer" [W2] then falls out of the existing
   newest-timestamp rule. Older rows stay in the namespace (insert-only) for
   calibration and history. A producer that writes no role/instance keeps
   today's per-session behavior, since its key is ("", "").
2. **Row ID includes role and instance:** SHA-256(session NUL role NUL
   instance NUL ts), for input rows that carry `"v": 2`. `hev score` always
   writes v2. Rows without `v` keep the old formula, so replaying an old file
   through `hev eval put` still addresses the same rows. Scorer rows use `role = "scorer"`, so their IDs
   stay disjoint from replayed historical rows, as
   [eval corpus replay](../runbooks/eval-corpus-replay.md) requires of any
   other writer.
3. **Optional `labels` object**, stored as `label_<name>` string columns.

`poor` filters then mean "any producer's latest row is poor", and a new
`instance` filter narrows it to one scorer. The label-review provider
contract already says "never deduplicate by session", so it needs no change.

### Q4. Where and when scoring runs, which judge, and who pays

**Recommendation: hevd schedules scoring on the host that holds the run.
Judges are headless runs of the operator's own harness CLIs, on their
subscriptions, with a daily cap.**

When:

| Trigger | What runs | Sampling |
|---|---|---|
| Dispatch run reaches a final outcome (`done`, `blocked`, `failed`, `timeout`, `needs-decision`), after any `finish` gates | Every enabled scorer whose `applies` matches | Rule: always. Judge: `sample` |
| hevd ingests a non-dispatch session (only with `[scorers] sessions = true`) | Scorers with `applies.runs` = `session` or `both` | `sample`, default 10 |
| The outcomes sweep patches a session (PR merged, CI completed, revert window closed) | Rule scorers that read outcomes | Always. A new row only when the label changes. |
| `hev score run` | Named scorers on named subjects | Unsampled |

Sampling is deterministic: a subject is in the sample when
`SHA-256(session_id NUL scorer) mod 10000 < sample × 100`. Re-running a
sweep therefore selects the same subjects, and different scorers sample
independently. Warp's docs say a 10% rate "evaluates about one in ten runs"
[W2] and do not say how runs are chosen.

Judging runs as a **judge job**, not a dispatch run. There is no worktree,
branch or PR. hevd builds the packet, writes it to a private temporary
directory, and invokes the harness headlessly with tools disabled:

- Claude Code: `claude -p` with an empty tool allowlist and the packet on
  stdin;
- Codex: `codex exec --sandbox read-only` with the packet on stdin.

The output is parsed as in Q2. The judge's harness and model are chosen by
the host in this order: `[scorers.<name>]` override, the scorer's `judge:`
request (only if the host allows the pair), `[scorers] judge_*`. Where the
host has more than one harness, the default judge pair differs from the
implementer's pair, following RFC 0019's validator rule.

Payment and accounting:

- Judges use the CLIs and logins the host already has, so they draw on the
  operator's existing subscriptions. A host with only an API key pays API
  rates. Nothing requires a hosted account.
- Judge sessions are captured like any session, tagged `role = judge` and
  `judge_of = <session>`, so [cost attribution](../cost-attribution.md)
  counts them. `hev score report` shows scoring cost on its own line, the
  separation Warp's benchmark totals make when they "include trials and
  Scorer usage" [W4]. Default trace views exclude `role = judge`.
- `judges_per_day` caps judge jobs per host. Past the cap, sampled subjects
  are recorded as `skipped: budget` in `hev score log`, not silently dropped.
- Judges are a dispatch class (`class: judge`, default 1 slot), so scoring
  never takes a worker slot from real work.

CLI:

```sh
hev score ls                                  # enabled scorers, kind, sample, calibration, stale pins
hev score run RUN [--scorer NAME]...          # score a dispatch run now (unsampled)
hev score run --session ID [--scorer NAME]
hev score backfill --scorer NAME --since 14d [--sample 25] --dry-run   # count and estimated cost first
hev score show RUN|--session ID               # latest result per scorer, with reasoning
hev score report [--since 30d] [--repo R] [--by harness,model] [--json]
hev score calibrate NAME                      # agreement with /review verdicts
hev score adopt REPO:NAME                     # pin a repository scorer at its current digest
hev score log [--errors]                      # judge errors, budget skips
```

`hev score report` is the first readout:

```text
kit, dispatch runs, 2026-10-19..2026-11-18 (212 runs)
                       codex/codex-model claude/opus-5.5
finished (done)        91/104  88%       97/108  90%
first-pass             41/104  39%       52/108  48%
acceptance-met (pass)  31/52   60%       40/55   73%   judge sample 50%, calibrated (κ 0.71, n=41)
pr-merged              80/91   88%       85/97   88%
kept (14d, complete)   71/74   96%       77/79   97%
ci-green (final head)  84/91   92%       90/97   93%
cost per kept PR       $3.10             $4.85         subscription-allocated, incomplete weeks excluded
scoring cost           $0.41 (judge, 107 jobs)
```

(Illustrative numbers.)

### Q5. Scorers and gates

**Recommendation: scorers never block. A gate can consume a scorer in two
explicit ways, and either way the gate is declared in `[[gates]]`.**

| | Scorer | Gate (RFC 0019) |
|---|---|---|
| When | After the fact, sampled | On a transition, every time |
| Failure | Fail-open: an error writes nothing | Fail-closed: `error` blocks |
| Bound to | A session, newest per scorer | A subject at a head SHA |
| Effect | An eval row, a report, an improve input | Rework, block or merge permission |
| Declared | `[scorers]` | `[[gates]]` |

Two ways a scorer feeds a gate:

1. **Reuse the rubric.** A validator gate names a scorer's file as its rubric
   (`rubric = "scorer:acceptance-met"`). The gate then runs the judge
   unsampled on every head, and maps labels to verdicts (`passing` →
   `pass`, otherwise `fail`). Its result is a gate verdict in `gates.json`,
   and it is *also* written as a score, so the two never disagree about the
   same head.
2. **Read a score.** A `command` gate runs `hev score check RUN --scorer
   ci-green --pass`. The command exits non-zero when the latest score is
   missing or failing. Since gates fail closed, a missing score blocks; that
   is the gate's choice, not the scorer's.

The reverse direction is the useful one in practice. Scorers measure gates.
`kept` and `ci-green` on merged runs show whether `merge` gates let
regressions through, and `acceptance-met` on runs that passed a validator
shows how often the validator was too lenient.

### Q6. Benchmarks

**Recommendation: a task is a dispatch order with a pinned base SHA. A suite
is a small TOML file. Trials are non-pushing dispatch runs, scored by every
listed scorer. Output is a CLI table first.**

Layout (in a repository, or in `~/.config/hev/benchmarks/`):

```
benchmarks/
  kit-docs/
    suite.toml
    tasks/
      kit-scorers-rfc.md
      kit-help-examples.md
```

`suite.toml`:

```toml
description = "Docs and RFC work on kit"
tasks = ["kit-scorers-rfc", "kit-help-examples"]    # order of the report
scorers = ["acceptance-met", "finished", "first-pass", "cost"]
repetitions = 3
host = "oven"                 # default trial host
timeout = "90m"               # per trial
budget_usd = 40               # refuse to start if the estimate exceeds it

[[config]]
name = "baseline"
harness = "claude"
model = "claude-opus-5-5"

[[config]]
name = "codex"
harness = "codex"
model = "codex-model"

[[config]]
name = "baseline+review-skill"
harness = "claude"
model = "claude-opus-5-5"
skills = ["shelf:review-before-done"]   # vary instructions, not just models
```

A task, built with `hev bench task from-run kit-scorers-rfc-20261118` from
this RFC's own order (body shortened here; the generated file copies it
verbatim):

```markdown
---
name: kit-scorers-rfc
repo: kit
base: 1780556e949bb78b48d5fc43e8c797a0a2a930fa
class: docs
from: oven/kit-scorers-rfc-20261118
reference: https://github.com/hev/kit/pull/284    # the original PR; never shown to trials or judges
tags: [rfc, docs]
timeout: 120
---
Write RFC 0021: Scorers, benchmarks and self-improvement for kit, adapted
from Warp Factories. Docs only, no code.

(… the order's Why, What Warp does, What kit already has and the ten
questions, copied verbatim …)

## Acceptance

- `docs/rfcs/0021-scorers.md` exists, follows the style of existing RFCs
  (status line, summary, motivation, decision, design, non-goals, rollout),
  and answers all ten questions above with a recommendation each.
- It includes a side-by-side table mapping each Warp concept (scorer field,
  benchmark field, self-improvement setting) to the kit equivalent or "not
  adopted".
- It includes one complete example scorer file in the proposed kit format and
  one example benchmark task derived from a real-shaped dispatch order.
- Every claim about Warp cites one of the sources above; nothing about Warp
  is invented. Where Warp's docs are silent, say so.
- `docs/rfcs/README.md` index updated if the README lists RFCs.
- No code changes; `git diff --check` passes.
```

Building tasks:

- `hev bench task from-run RUN` copies `order.md` and resolves `base` to the
  full SHA the run's branch started from. Dispatch creates `d/<name>` at
  `origin/<base>`, so this is the branch's fork point; dispatch should also
  record the SHA in `meta.json` at admission, which it does not today. It keeps `## Acceptance`, records
  `from:` and the run's PR as `reference:`, and drops run-specific frontmatter
  (`host`, `reply`, `harness`, `model`) that configurations set instead.
  Acceptance items that are about delivery rather than the work ("draft PR
  opened", "push the branch") are moved to a `## Not benchmarked` section,
  because trials do not push.
- `hev bench task from-session ID` starts from a captured interactive
  session: the first user prompt becomes the body. The session's recorded
  `workdir` gives `repo`, and the parent of the first commit in its
  `commits` ([session git](../session-git.md)) gives `base`. A session with
  no commits has no recoverable start state, so `base` must be given with
  `--base SHA`. It writes `## Acceptance` as a `TODO` and refuses to run
  the task until a person fills it in. An interactive prompt is not a
  specification, and an invented acceptance list would measure nothing.
- Tasks must have a full 40-character `base`, as Warp requires for
  `startingRepoRefs` [W5].

Running:

- `hev bench run kit-docs [--config NAME]... [--reps N] [--dry-run]` expands
  tasks × configs × repetitions into orders named
  `bench-<suite>-<task>-<config>-<rep>`, class `bench`. It prints the trial
  count and a cost estimate from the source runs' priced cost, and stops if
  `budget_usd` would be exceeded.
- Trials are ordinary dispatch runs with a new order field, `publish: none`.
  Dispatch tells the worker not to push or open a PR, and the supervisor
  refuses `git push` for the run (with RFC 0019 tool gates, a `git.push`
  deny). The trial's
  worktree head is the result.
- **Contamination.** A task built from a past run has its answer in the
  repository's later history. A trial worktree is a fresh clone at `base`
  with `--single-branch` and no later refs, so `git log` cannot show the
  reference PR. The task's `reference` is never put in the trial's context or
  the judge's packet. Web access to the repository is still possible, so
  report a trial whose transcript fetches the reference PR URL as
  `contaminated`.
- Every trial is scored by every scorer the suite lists, unsampled.
  `blocked` and `needs-decision` trials are scored on what they produced, as
  in Warp [W4]. A `needs-decision` trial is answered by nobody: it times out
  as `blocked`, because a benchmark measures unattended work.

Output (`hev bench report kit-docs [RUN-ID] [--json]`):

```text
kit-docs run 2026-11-20T10:02Z  2 tasks × 3 configs × 3 reps = 18 trials
                          baseline        codex           baseline+review-skill
acceptance-met (pass)     4/6             3/6             6/6
  kit-scorers-rfc         2/3             1/3             3/3
  kit-help-examples       2/3             2/3             3/3
finished (done)           6/6             5/6             6/6
first-pass                n/a (no reviewer in trials)
cost / trial (median)     $2.10           $1.40           $2.60
time / trial (median)     31m             24m             38m
scoring cost              $0.22

Best acceptance: baseline+review-skill (6/6 vs 4/6; n=6 per config, not significant at p<0.05)
Cheapest passing config: none clearly; codex is cheaper but passes 3/6
```

(Illustrative numbers.)

- Recommendations come with their n, and say "not significant" when a
  two-sided Fisher exact test on pass counts gives p ≥ 0.05. Warp's docs say
  the overall recommendation needs "comparable results" but not how that is
  decided [W4]. Kit states its test.
- Results are frozen in `~/.hev/bench/<suite>/<run-id>/results.json` with the
  SHA-256 of the suite, each task and each scorer file, and every trial's run
  name and eval row IDs. Changing a scorer later does not change past
  results, as in Warp [W4]. `hev bench rescore RUN-ID --scorer X` writes a new
  result set beside the old one rather than editing it.
- A dashboard view comes after the CLI is used for a while (phase 4).

### Q7. Self-improvement through the improve orchestrator

**Recommendation: add a `scores` source to orchestrators. The RFC 0019
improve orchestrator subscribes to it, and turns grouped failures into one
dispatch order per cause. The PR targets code, skills or instructions, and a
person merges it.**

The source emits an event when, for one (scorer with `improve = true`, repo):

- open failures ≥ `threshold` (default 10, range 1–50, matching Warp's range
  [W3]), or
- the oldest open failure is older than `max_age` (default 7d, as Warp's
  default [W3]) and there are at least 3 open failures.

Kit's default threshold is lower than Warp's 25 [W3] because one operator's
cluster produces tens of dispatch runs a day, not hundreds (the trace review
read 355 non-client runs in 17 days).

```toml
[scorers.improve]
enabled = true
orchestrator = "improve"      # the applied orchestrator spec that receives events
threshold = 10
max_age = "7d"
```

**Open and addressed.** Warp counts "unreviewed failures" but does not
define reviewed [W3]. In kit, a failure is open until one of these:

- it is listed in an improvement PR's "Regressions addressed" section while
  that PR is open or merged (if the PR closes unmerged, its failures reopen);
- a person marks the label `incorrect` in `/review`. The judge was wrong,
  which counts against the scorer's calibration, not the run;
- a newer score for the same subject passes (rescoring after a rework).

The orchestrator round (RFC 0008: gather → act → checkpoint):

1. **Gather** (code): open failures for the scorer and repo, each with its
   label, judge reasoning, findings, the order's name and the run's outcome.
2. **Act** (model): group failures by cause, as RFC 0019's improve
   orchestrator already proposes for gate verdicts. For each recurring cause
   with ≥ 3 failures, dispatch one order. The order targets the smallest
   change that would have prevented it:
   - an AGENTS.md or CLAUDE.md line, a context recipe, or a shelf skill;
   - application code or tests;
   - **the scorer itself**, when the failures are judge mistakes (the
     rubric is ambiguous, or the packet lacked an input). For a repository
     scorer the PR edits `.hev/scorers/`; adopting it still needs a host
     pin. For a host scorer, the order produces a proposed diff in its
     report for a person to apply;
   - **a gate**, proposed as a `hev host config` diff in the report. It
     never edits host config (RFC 0019).
3. **Checkpoint:** which failures each order covers, so the next round does
   not dispatch them again.

The improve order's Acceptance always includes:

- The PR body has a `## Regressions addressed` section. It is a table of the
  failing runs (`host/run`, session ID, scorer, label, one-line reasoning)
  that this change is meant to prevent, generated by `hev score
  regressions --scorer X --cause ID`.
- When a benchmark suite covers the repo, the PR reports `hev bench run`
  results for a suite built from the failing runs (`hev bench task from-run`
  for each), before and after. This is optional until phase 3 is
  dogfooded.

The orchestrator has no merge authority (RFC 0008). Its requester is
requested as reviewer. There is no "no reviewer" setting like Warp's
`reviewerType: none` [W5].

### Q8. Built-in scorers

**Recommendation: ship six rule scorers and three judges. Enable the rule
scorers and `acceptance-met` by default on hosts that run dispatch.**

Rule scorers (free, unsampled, from records kit already keeps):

| Scorer | Labels (score) | Reads | Note |
|---|---|---|---|
| `finished` | `done` (1), `needs_decision` (0.5), `blocked` / `failed` / `timeout` / `stopped` (0) | `result.json` outcome | |
| `first-pass` | `first` (1), `reworked_once` (0.5), `reworked` (0) | `history.json` rework count, excluding metadata-only reworks | Scope changes by the owner count as reworks. The cause split is a finding, not a label. |
| `pr-merged` | `merged` (1), `closed_unmerged` (0); `open`, `no_pr` → unscorable | outcomes `pr_state` | Rescored when outcomes change |
| `kept` | `kept` (1), `reverted` (0); `observing` → unscorable | outcomes `reverted`, `revert_state` | Only `complete` or `observed` revert states score |
| `ci-green` | `green` (1), `red` (0); `pending`, `none` → unscorable | outcomes `ci_conclusions` at the final head | `skipped`/`neutral` do not count as red or green |
| `cost` | No labels; writes the integer mark `cost_cents` | Priced cost from `hev cost-price` | Never `poor`. Feeds "cost per kept PR" in reports. |

"Cost per merged PR" and "cost per kept PR" are aggregates in `hev score
report`, not per-run scores: total priced cost of runs (including reworks and
judges) divided by runs with `pr-merged = merged` or `kept = kept`. As in
[cost attribution](../cost-attribution.md), unknown prices make the figure
incomplete, not zero. Warp likewise calls its cost per PR "a comparison
metric, not a billing figure" [W1].

Judge scorers (sampled):

| Scorer | Question | Why |
|---|---|---|
| `acceptance-met` | Was every Acceptance item met and shown, and only what was asked? (the example in Q1) | Kit orders always carry Acceptance. This is kit's counterpart to Warp's benchmark Correctness scorer [W4] and its example `task-compliance` [W6]. |
| `honest-report` | Do `report.md` and the final status match the evidence? (tests claimed as run were run; CI claimed green was green; flaky tests reported) | Five runs in the trace review reported done with CI red or a check not run. |
| `scope` | Did the run change only what the order asked for? | Split out of `acceptance-met` when a host wants it separately. Off by default. |

Not built in: verbosity or style scorers. The trace review found no failure
they would catch, and the recommended loop is one scorer per observable
failure mode [W1].

### Q9. Rollout

**Phase 1: rule scores and one judge, dogfooded on Adam's cluster.**

- RFC 0005 amendment: latest per (session, role, instance), the new ID
  formula for producer rows, and `labels`.
- Rule scorers `finished`, `first-pass`, `pr-merged`, `kept`, `ci-green` and
  `cost`. They run on dispatch finish and on outcome sweeps.
- The `acceptance-met` judge on dispatch finish at `sample = 100` for kit
  runs on oven and mini. Built-in rubric only; no scorer files yet.
- `hev score run|show|report|log|backfill`.
- Scorer evals as a label-review kind; `hev score calibrate`.
- **Exit criteria, after two weeks:** rule scores on ≥ 90% of finished
  dispatch runs; 40 human verdicts on `acceptance-met` with Cohen's κ ≥ 0.6;
  judge cost below 5% of worker cost on the same runs.

**Phase 2: scorer files and benchmarks.**

- Scorer files, `[scorers]` enablement, repository proposals pinned by
  digest, `honest-report`, sampling for captured sessions (opt-in).
- `hev bench task from-run|from-session`, `hev bench run|report|rescore`,
  and `publish: none` trials with contamination-safe clones.
- First suite: ten kit orders from the trace review window that merged and
  stayed merged, against the two harnesses Adam uses.

**Phase 3: self-improvement.**

- The `scores` orchestrator source, open/addressed tracking, `hev score
  regressions`, and improve orders with "Regressions addressed".
- Lands after RFC 0019's improve orchestrator, and after `hev host config`
  if scorers are to be protected from runs.

**Phase 4: dashboard.** Scorer columns and filters (they come mostly free
from RFC 0005), a benchmark results page, and scorer cards.

### Q10. What kit does not copy from Warp, and why

- **A hosted control plane and account.** Warp's definitions can be
  Warp-managed, or applied when merged to a GitHub repository on paid plans
  [W5]. Runs and metrics live in Warp's web app [W5]. In kit everything runs
  on the operator's hosts, and results land in the operator's Layer
  namespace.
- **Repository-authoritative scorers.** Merging a definition applies it in
  Warp [W5]. Kit pins repository scorers by digest on the host (Q1), because
  the scored agents write to those repositories.
- **LLM-only scoring.** Warp's scorers are LLM judges [W2]. Kit starts from
  deterministic rule scorers and adds judges where no record answers the
  question.
- **Named agents as the unit.** Warp scorers and suites target agents
  [W5]. Kit has orders, classes, repos and harness/model pairs, and selects
  on those.
- **`runner`, `secrets` and `mcpServers` on scorers** [W5]. A kit judge has
  no tools and no secrets; it reads a packet.
- **Opaque judge inputs.** Warp's docs don't say what the judge sees [W2],
  [W5]. Kit declares inputs and budgets per scorer, and records what was
  truncated.
- **A combined "Overall" value.** Warp's results table shows "each Scorer's
  average and the combined Overall value" [W4]. Kit reports each scorer
  separately, because a blend hides which question got worse.
- **Recommendations without stated statistics.** Warp says "comparable
  results" without defining it [W4]. Kit prints n and its test.
- **Self-improvement edits to the control plane.** Warp's follow-up runs can
  open PRs against the factory definition [W3]. Kit's improve orders never
  touch host config or gates; they propose a diff for a person.
- **`reviewerType: none`** [W5]. Every improvement PR requests a person.
- **A configuration limit of six** [W5]. Kit prints a cost estimate and
  enforces `budget_usd` instead.
- **Benchmark repo substitution** (`benchmarkRepoSubstitutions`) [W5]. Kit
  tasks name a repo the trial host already has.

## Open questions

1. **Eval namespace amendment vs a new namespace.** This RFC amends RFC 0005
   (latest per producer). The alternative is a separate `-scores` namespace
   with its own schema, which leaves 0005 untouched but splits the dashboard
   filters. The amendment looks smaller. Is changing the join acceptable for
   existing eval producers?
2. **Judge for interactive sessions.** Should `[scorers] sessions = true`
   ever be on by default? Scoring Adam's own sessions is useful for "what
   went wrong today", but it reads private conversations with a model.
3. **Cross-host scoring.** A run on win-oven (client work) must be scored on
   win-oven or not at all. Is a per-host `judge_host` enough, or does the
   client boundary need a rule that judges never leave the run's host?
4. **Benchmark trial isolation.** A fresh single-branch clone hides later
   history. Is that enough, or should trials also deny `web.fetch` to the
   repository's own GitHub URL by default?
5. **Calibration bar.** Is κ ≥ 0.6 on 40 verdicts the right bar before a
   judge can feed improve?
6. **Who is the reviewer of improve PRs** when the improve orchestrator was
   started by an orchestrator rather than a person: the host owner, or the
   requester of the orchestrator's parent?

## Sources

Read 2026-11-18. Quotes are from the Markdown versions of the docs pages
(the page URL with `.md` appended).

- [W1] Warp, "Measure and improve a factory",
  https://docs.warp.dev/factories/measure-and-improve/
- [W2] Warp, "Configuring Scorers",
  https://docs.warp.dev/factories/measure-and-improve/scorers/
- [W3] Warp, "Configuring and reviewing Self-improvement",
  https://docs.warp.dev/factories/measure-and-improve/self-improvement/
- [W4] Warp, "Benchmarking factory agent configurations",
  https://docs.warp.dev/factories/benchmarks/
- [W5] Warp, "Factory definition syntax" (definitions as code),
  https://docs.warp.dev/factories/factory-as-code/
- [W6] warpdotdev/warp-factory-examples (MIT),
  https://github.com/warpdotdev/warp-factory-examples, in particular
  `examples/02-sdlc-issue-to-pr/scorers/task-compliance/scorer.md`
- [W7] Ben Holmes, "Terraform for agents: how we define our software factory
  in code", Warp blog, 2026-09-30,
  https://www.warp.dev/blog/software-factory-as-code

Where the sources disagree, this RFC follows the docs. The blog describes
benchmark tasks as Markdown files and self-improvement as running on a
schedule [W7]. The docs describe tasks as YAML [W5] and the trigger as 25
failures or 7 days [W3].
