# kit-summary-source: rework guidance v2 (FOH, 2026-10-29)

Two rounds of text heuristics have failed adversarial validation. Change approach; keep the rest of 8824d28885e1f3b90e2364cf8cea7ae1b3b5f9bf (origin branch candidates/kit-summary-source-20261029-r3).

1. Select the request by transcript structure, not text patterns. Each harness parser already sees message items: Codex injects AGENTS.md/user_instructions, environment_context and permission/sandbox text as separate items or tagged blocks before the user's message; Claude Code wraps system-reminders and pasted/peer content in tags. Mark those injected items/blocks at parse time (by item type, tag or position) and pick the first user-authored item that is not injected. Keep a small text fallback only for archives that lost structure, and make it strip whole known blocks (including Markdown bullet and backtick forms such as "- `sandbox_mode` is ..."). Injected-only input must yield an empty selection.
2. Replace the answer guard with a model contract: instruct Haiku to reply exactly NO_TASK when the input contains no concrete user task (only instructions, environment, permissions or coordination setup). Treat NO_TASK, an empty answer, or an answer failing a minimal sanity check as not done with a retriable reason. Do not try to detect preamble descriptions by word overlap.
3. Add both validator probes below as permanent tests, plus the earlier probes from round one.

## Validator findings, run :4
Independent validation: FAIL
Candidate: 8824d28885e1f3b90e2364cf8cea7ae1b3b5f9bf
Context digest: 98ec0d43c933ca9d131850e40057104939241eaf6329ab77d9fc175eed0b85e0

Baseline: git diff --check passed (no output); go test ./... passed (all packages).
The checked-in normalized fixtures exercise Codex, Claude Code, PI and dot,
including user/assistant/tool fallback, peer and pasted wrappers. Docs describe
selection and the guard. No live namespace writes, credentials, deployment or
acceptance were performed.

Two temporary independent Go tests in cmd/hev reproduced remaining defects with
`go test ./cmd/hev -run '^TestIndependent' -count=1` (exit 1). Temporary tests were
removed after execution; candidate implementation was not changed.

1. Call summarizeArchiveSession with FirstPrompt `Fix dashboard summaries` and a
model double returning `Dashboard session: agent coordination instructions and
environment setup`. Observed SummaryBackfilled=true and that exact summary.
Expected rejection with completion false. Root cause: PreambleSummary accepts any
shared content word, including incidental dashboard/session labels; the answer
still exclusively describes harness setup. This violates the answer guard.

2. Pass the following injected-only text to trace.RequestText:

    Filesystem sandboxing defines which files can be read or written.
    - `sandbox_mode` is `danger-full-access`: No filesystem sandboxing - all commands are permitted.
    Network access is enabled.
    Approval policy is currently never.

Observed return value (nonempty):

    - `sandbox_mode` is `danger-full-access`: No filesystem sandboxing - all commands are permitted.
    Network access is enabled.
    Approval policy is currently never.

Expected empty selection, so the archive path falls back to later requests or
stores the injected-context reason with completion false. Root cause:
permissionLine accepts plain colon/equal metadata but not the actual Markdown
backtick and prose form; stripping stops on that line. This can feed the same
injected context to the model or dashboard title.

Observed probe output:
--- FAIL: TestIndependentPreambleOverlap (0.00s)
    preamble-only answer marked done: summary="Dashboard session: agent coordination instructions and environment setup" backfilled=true
--- FAIL: TestIndependentPermissionBullets (0.00s)
    injected-only permission preamble retained as request: "- `sandbox_mode` is `danger-full-access`: No filesystem sandboxing - all commands are permitted.\nNetwork access is enabled.\nApproval policy is currently never."
FAIL github.com/hev/kit/cmd/hev

Verdict: fail. Preserve candidate for rework; do not accept.
