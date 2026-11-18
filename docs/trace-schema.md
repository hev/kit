# The trace envelope

Every event kit captures is wrapped in one trace envelope before it is
archived. The canonical column reference is
[`schemas/trace_envelope.sql`](../schemas/trace_envelope.sql); the Go type is
`daemon.TraceEnvelope`. The envelope shape never changes;
`payload_schema_ref` versions what is inside `payload_raw`.

| Field | Notes |
|---|---|
| `ingest_id` | ULID; row identity |
| `ingest_ts` | when kit saw the event |
| `ingest_source` | `claude_raw_api_body_file`, `codex_rollout_jsonl`, `otlp_http`, ... |
| `ingest_agent`, `ingest_agent_version` | `hev-kit` and its build SHA |
| `source_ts` | the event's own timestamp |
| `host`, `gh_user` | the machine, and the GitHub login where known |
| `harness` | `claude_code`, `codex_cli`, or `app:<name>` for a host app |
| `harness_version` | e.g. `codex-0.130.0` |
| `session_id` | the join key across one session's events; for an app, the chat id |
| `turn_id` | per-turn id; set by app capture |
| `event_kind`, `signal_type` | harness-native event name; `trace`, `log`, ... |
| `payload_raw` | the native event after local policy and redaction |
| `payload_schema_ref` | e.g. `codex.rollout_jsonl.v0.130.0`, `otel.span.ai-sdk.v1` |
| `redaction_policy`, `redaction_applied`, `redaction_count` | set by app capture, which scrubs each span |
| `resource` | JSON map of resource attributes |
| `repo`, `branch` | denormalized from the resource where known |

## Harness values

- `claude_code`, `codex_cli`: coding-agent capture (`hev up`, `hev d`).
- `app:<name>`: app capture (`hev collect`), where `<name>` is the OTel
  resource `service.name` of the host app, e.g. `app:ki-agent`. See
  [app traces](app-traces.md).

## Resource fields

`resource` carries the OTel resource map (or its equivalent for non-OTLP
sources). App capture adds:

| Key | Meaning |
|---|---|
| `end_user` | the host app's user who owns the turn (`ai.telemetry.metadata.userId`) |
| `tenant` | the host app's tenant (`ai.telemetry.metadata.tenant`) |
| `chat_id` | the conversation (`ai.telemetry.metadata.chatId`) |

A span that never found its turn is written without `end_user`, `tenant` and
`turn_id`.
