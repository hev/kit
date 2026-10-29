# kit-summary-source: rework guidance v4 (FOH, 2026-10-29)

Start from ebbd7d889ed102012ab0c466256a59554e8d9118 (origin branch candidates/kit-summary-source-20261029-r7). Keep its request selection, JSON contract and no-task rules. Run :8 failed on one gap: valid JSON with task_present=true whose summary only describes setup is still marked done.

## Add a grounding check (not paraphrase matching)

Define one list of setup vocabulary in one place: AGENTS.md, CLAUDE.md, instructions, system prompt, preamble, sandbox, permission(s), approval policy, environment, environment context, coordination, agent-coordination, peer message, configuration/config, setup. Match case-insensitively on word stems.

Reject the answer (blank summary, summary_backfilled=false, retriable reason "summary describes setup, not the request") when the summary uses any setup term that does NOT appear in the selected request text. When the request itself is about such things (e.g. "update AGENTS.md sandbox rules"), the same words are grounded and the summary is kept.

This rule is deterministic and checkable: every setup term in a stored summary must be grounded in the request. It replaces any remaining phrase-list PreambleSummary logic besides the explicit no-task rules from guidance v3.

## Required tests (permanent)

With FirstPrompt "Repair dashboard summaries" and model JSON {"task_present":true,"summary":S}, each S must be rejected:
- hevmind-promo session: archived prompt is only the agent-coordination preamble
- Only AGENTS.md instructions and sandbox context
- Sandbox permissions and environment configuration
- Dashboard session: agent coordination instructions and environment setup
- Read AGENTS.md and CLAUDE.md instructions; no user task provided

Must be kept (summary_backfilled=true):
- FirstPrompt "Update AGENTS.md sandbox permission rules for Codex", S "Updated AGENTS.md sandbox permission rules"
- FirstPrompt "Repair dashboard summaries", S "Repaired dashboard summary selection"

Also keep all existing tests. Fix the old plain-text preamble fixture in backfill_summary_test.go so it goes through the JSON path. Update docs/session-backfill.md: list the grounding rule, the vocabulary, and the residual risk, which FOH audits live after rollout. Commit only, no live writes.

## Validator findings, run :8
Independent validation: FAIL
Candidate: ebbd7d889ed102012ab0c466256a59554e8d9118
Context digest: 8ae345ddd61c037d771d28b21004a7b71a2e0125f08f23cd03c8104c9ef04c9f
Order: kit-summary-source-20261029
Order UID: 523a1477fe22bdc141466550659f58c7
Validator run: 523a1477fe22bdc141466550659f58c7:8

Required checks on exact candidate:
- git diff --check: PASS (exit 0; no output)
- git diff ef1ef66 HEAD --check: PASS (exit 0; no output)
- go test ./...: PASS (exit 0; all packages pass)
- Fixture tests cover injected first prompt followed by request, peer/pasted payloads,
  assistant/tool fallback and context-only reason/incomplete state.
- Documentation describes source selection and structured answer contract.
- No provider invocation, credential resolution, deployment, push, acceptance or live
  namespace writes performed. Tests use local doubles. Artifact registration is the
  explicitly requested validation control-plane write.

Blocking finding: preamble-only answers in valid model JSON are marked done.
internal/trace/summary_source.go PreambleSummary only recognizes explicit absence
of a task (e.g. "no task"). It does not reject descriptions of instructions or
preamble such as the exact known rollout failure. task_present=true is trusted
for those descriptions. The preamble guard fixture in backfill_summary_test.go
passes plain text, which now fails JSON parsing before exercising this guard.

Independent reproduction used Go -overlay to append a test to the candidate's
backfill_summary_test.go without modifying candidate source. For each summary:
  hevmind-promo session: archived prompt is only the agent-coordination preamble
  Only AGENTS.md instructions and sandbox context
  Sandbox permissions and environment configuration
marshal {"task_present":true,"summary":summary}, pass it to
summarizeArchiveSession with FirstPrompt="Repair dashboard summaries", and assert
summary remains blank and SummaryBackfilled remains false.
Command: go test -overlay /tmp/kit-summary-validation-523a1477/overlay.json ./cmd/hev -run TestIndependentValidationStructuredPreamble -count=1
Observed Go test exit: 1 (the shell printing its captured output exited 0).
All three cases failed: SummaryBackfilled=true, SummaryReason empty.

Required rework: enforce the preamble-only answer rejection within the structured
contract, preserving real tasks about instruction files; test valid JSON envelopes
with these known bad answers. Candidate fails the explicit guard acceptance item.

Captured independent test output:
--- FAIL: TestIndependentValidationStructuredPreamble (0.00s)
FAIL
FAIL	github.com/hev/kit/cmd/hev	0.446s
FAIL
