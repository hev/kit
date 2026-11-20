# Degrade limit fixtures

Inputs for `TestClassifyLimit*` (`../../degrade_test.go`). Session, request and
message IDs are replaced with zero UUIDs and working directories with
`/redacted`; nothing else was changed.

Captured (positive cases):

- `claude-stream-usage-limit.jsonl`: the last three stream-json records of
  `follow-20261119-034402` round 047 on mini (`rounds/047/trace`,
  2026-11-19), the incident that failed 47 rounds: a rejected
  `rate_limit_event`, the synthetic assistant error (`api_error:
  usage_limit_reached`, `api_error_params.rate_limit_info`, `error:
  rate_limit`, 429 `rate_limit_error`) and the `is_error` result.
- `claude-transcript-usage-limit.jsonl`: the same error as Claude Code's
  transcript writes it (`apiError`, `apiErrorParams`, `quotaLimits`), from that
  orchestrator's round 001 session file on mini.
- `codex-rollout-usage-limit.jsonl`: a Codex rollout `task_complete` whose
  `error.codex_error_info` is `usage_limit_exceeded`, with the `token_count`
  credits record before it. Recorded on mini on 2026-09-14 (a factory job's
  Codex sessions; quoted in the hev traces archive, session
  01a0a13c-e02e-7762-948c-ac7164b55beb, and a macbook session that dumped the
  raw rollout line). The rollout files themselves have since been pruned.
- `codex-exec-usage-limit.jsonl`: what `codex exec --json` (the form dispatch
  logs) prints for that failure. The envelope, `{"type":"error","message"}`
  then `{"type":"turn.failed","error":{"message"}}`, is copied from a real
  failed turn in an oven dispatch log (`codex-exec-cyber-policy.jsonl`); exec
  does not carry `codex_error_info`, so the message is the evidence. The message
  is the recorded one above; the `You've hit your usage limit` strings and the
  `usage_limit_exceeded` CodexErrorInfo variant are in the codex 0.170.0
  binary.

Negative cases (must not classify as a limit):

- `codex-exec-cyber-policy.jsonl`: captured from an oven dispatch log
  (kit-07-files-board-review-20261117), a policy refusal.
- `codex-exec-capacity.jsonl`: Codex's model-capacity failure, as recorded in
  the same factory fixture set.
- `claude-overloaded-529.jsonl`: a 529 `overloaded_error`. No 529 record was
  found on oven, mini or in the traces archive, so this one is assembled from
  the captured error record's shape with the overload status and type.
- `claude-test-failure.jsonl`: a worker whose tests fail; the limit message
  appears only inside tool output and model text, never as an API error.
- `claude-crash.jsonl`: a harness that crashed mid-stream.

A timeout is a supervisor fact, not a log record; the tests cover it through
`limitFromRun`.
