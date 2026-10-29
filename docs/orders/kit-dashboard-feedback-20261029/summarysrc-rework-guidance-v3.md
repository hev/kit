# kit-summary-source: rework guidance v3 (FOH, 2026-10-29)

Start from 2a5e7f0a011e0b08416366cbdb143f78358820cb (origin branch candidates/kit-summary-source-20261029-r5). Its request selection passed independent validation; keep it unchanged. Only the answer guard failed (run :6, below).

## Acceptance interpretation (Adam's decision, 2026-10-29)

Adam narrowed the criterion "A preamble-only model answer is rejected and not marked done". A text rule cannot recognize every possible paraphrase of a setup-only description, so the criterion is NOT open-ended paraphrase detection. For this order it means exactly this. The model answer is rejected (summary blank, summary_backfilled=false, retriable reason recorded) when any of these holds:

1. The answer is the NO_TASK sentinel, or the structured output says there is no task (see below).
2. The answer is empty or whitespace, or it cannot be parsed under the output contract.
3. The answer states that there is no task or request. Examples: "no user task", "no task provided", "no request", "no concrete task", "nothing to do", "no instructions from the user", in any case and with any punctuation.

Remaining live risk, such as a fluent description of setup that never says "no task", is handled by FOH auditing the live repair after rollout. Validators should judge against rules 1–3 and the probes below, not invent new paraphrases outside them.

## Required changes

- Output contract: ask Haiku for one line of JSON, {"task_present": true|false, "summary": "..."}. task_present=false means rejected under rule 1. Keep accepting a bare NO_TASK reply. Decide how to handle a plain-text non-JSON reply and document it; rule 3 must still apply.
- Implement rule 3 as a small explicit pattern list in one place, with a unit test for each listed form.
- Add the run :6 reproducer below as a permanent test. Also add tests for: NO_TASK, empty output, task_present=false with a non-empty summary, unparseable output, and a normal valid summary still accepted with summary_backfilled=true.
- Remove or narrow the old phrase-based PreambleSummary so the behavior follows rules 1–3 alone. Update docs/session-backfill.md to state the rules and the residual risk.
- Commit only. No live writes.

## Run :6 reproducer (must pass)

```go
package main
import (
 "testing"
 "github.com/hev/kit/internal/trace"
)
func TestIndependentValidationPreambleOnly(t *testing.T) {
 answer := "Read AGENTS.md and CLAUDE.md instructions; no user task provided"
 row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair dashboard summary selection"}, func(trace.SessionRow) (string,error) { return answer,nil })
 if row.SummaryBackfilled || row.Summary != "" { t.Fatalf("preamble-only answer marked done: summary=%q summary_backfilled=%v reason=%q",row.Summary,row.SummaryBackfilled,row.SummaryReason) }
}
```

## Validator findings, run :6
# Independent validation: FAIL

Order: kit-summary-source-20261029
Order UID: 523a1477fe22bdc141466550659f58c7
Candidate inspected: 2a5e7f0a011e0b08416366cbdb143f78358820cb
Context digest: 4d4b977818814b2a6f8d1fac0623c54c853e731ebc07f2b44a23ec2c1fcec02c
Validator run: 523a1477fe22bdc141466550659f58c7:6

The candidate does not meet the acceptance criterion that a preamble-only model
answer is rejected and not marked done.

## Blocking finding

`internal/trace/summary_source.go:PreambleSummary` recognizes a narrow list of
setup phrases. A model answer saying `Read AGENTS.md and CLAUDE.md instructions;
no user task provided` does not match those phrases. Via the actual
`cmd/hev/backfill_summary.go:summarizeArchiveSession` path, it is stored unchanged
with `SummaryBackfilled=true` and an empty reason. This describes only the
instruction preamble and explicitly says there is no task. The NO_TASK prompting
contract does not enforce rejection when a model returns this natural-language
answer instead of the sentinel.

The independent reproducer is preserved in `guard-reproducer.go.txt`, and its
actual failing test output is in `guard-output.txt`. To reproduce against the
candidate, copy the text file to `cmd/hev/validation_summary_guard_test.go` and run
`go test ./cmd/hev -run TestIndependentValidationPreambleOnly -count=1 -v`.
Expected: blank summary, false completion. Observed: nonblank summary, true
completion. The temporary test was removed after execution; no implementation
files were modified.

## Other observed results

- `git diff --check`: exit 0, no output (`diff-check.txt`).
- `go test ./...`: exit 0, all packages pass (`go-test.txt`).
- Checked-in fixture tests cover normalized Codex, Claude Code, PI, dot,
  peer/pasted wrappers, injected-only prompts, and assistant/tool fallbacks.
- The candidate selects request payloads and applies selection to derived
  short titles; docs/session-backfill.md describes selection and the guard.
- The checked-in guard tests pass but omit the demonstrated model response.

No push, merge, deployment, live namespace write, credential resolution, worker
spawn, or acceptance was performed. Artifact registration is validation evidence
only. The implementation candidate remains the exact SHA above; the separate
findings commit contains only this report and its local validation evidence.
