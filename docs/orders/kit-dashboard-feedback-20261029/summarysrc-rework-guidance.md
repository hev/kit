# kit-summary-source: rework guidance (FOH, 2026-10-29)

Start from 0412c52cbca2a19642c92ce4b58e185322671daa (origin branch candidates/kit-summary-source) and keep its selection, unwrapping, fallbacks and tests. Fix the two validator findings below and add both probes as permanent tests.

1. Preamble-only answer guard: classify the model answer, not just look for "only"/"no request". Reject (do not mark done; store a retriable reason) any answer that describes agent/coordination/environment/sandbox/permission/instruction setup instead of work, for example "Agent coordination instructions and environment setup". Prefer a small deny-list of setup topics plus a check that the answer shares no content words with the selected request text, and test both directions (a real summary that mentions "agents" as subject matter must still pass).
2. Permission/sandbox preambles: strip the preamble block (Filesystem sandboxing..., sandbox_mode:, approval_policy:, network access lines, etc.) and keep the text after it. Never discard a whole prompt because it starts with a known preamble.

## Validator findings
# Independent validation: fail

Candidate: 0412c52cbca2a19642c92ce4b58e185322671daa
Context digest: 17ea02452d56f3e618e9935df7989fe870a5249b87e8584e8f766f172aacb4dc
Order: kit-summary-source-20261029 (523a1477fe22bdc141466550659f58c7)

The candidate adds normalized prompt selection, later-user and assistant/tool fallback,
peer/pasted wrapper stripping, title refresh, tests and documentation. Existing
fixture coverage passes, including Codex context then request, Claude peer payload,
PI pasted payload, dot request, context-only and assistant/tool fallback.

## Blocking findings

1. internal/trace/summary_source.go PreambleSummary requires an additional marker
such as "only" or "no request". A model answer "Agent coordination instructions and
environment setup" describes only preamble but passes the guard. Calling
summarizeArchiveSession with FirstPrompt "Repair the dashboard summaries" and a
stub returning that answer produces summary_backfilled=true, summary equal to that
answer, and an empty reason. This violates the preamble-only answer acceptance check.

2. RequestText discards the entire prompt when it starts with "Filesystem
sandboxing", even when a real request follows the permission preamble. Reproducer:

```text
Filesystem sandboxing defines which files can be read or written.
sandbox_mode: danger-full-access
approval_policy: never

Fix archive summary generation.
```

Observed RequestText output is empty; expected "Fix archive summary generation."
Thus summary repair may skip the first real request or record context-only when
request evidence exists in the same prompt.

## Verification

- git diff --check: exit 0, no output.
- git diff HEAD^ HEAD --check on candidate: exit 0, no output.
- go test ./...: exit 0, all packages passed (two packages have no test files).
- Additional temporary tests in cmd/hev called the actual candidate functions;
  go test ./cmd/hev -run '^TestIndependent' -v: exit 1. Tests were removed after
  execution; implementation files were not changed.

Exact independent probe output:

```text
=== RUN   TestIndependentPreambleAnswer
    validation_probe_test.go:9: answer="Agent coordination instructions and environment setup" summary="Agent coordination instructions and environment setup" summary_backfilled=true reason=""
    validation_probe_test.go:10: preamble-only model answer marked done
--- FAIL: TestIndependentPreambleAnswer (0.00s)
=== RUN   TestIndependentPermissionThenRequest
    validation_probe_test.go:15: selected=""
    validation_probe_test.go:16: real request after permission preamble lost
--- FAIL: TestIndependentPermissionThenRequest (0.00s)
FAIL
FAIL	github.com/hev/kit/cmd/hev	0.441s
FAIL
```

The first probe used summarizeArchiveSession(trace.SessionRow{FirstPrompt:
"Repair the dashboard summaries"}, stub) and asserted SummaryBackfilled=false.
The second used trace.RequestText on the input above and asserted preservation
of the request. No model, credentials, live namespace, deploy, push, merge or
acceptance operations were used. The durable inbox was empty at the checked
safe boundaries. This report records a failed candidate, not order acceptance.
