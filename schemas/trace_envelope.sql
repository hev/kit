-- Raw trace envelope.
--
-- Logical schema for the immutable archive of every event from every source
-- (OTLP signal from Claude Code and Codex, hook traces, Codex rollout JSONL).
-- Physical storage is JSON/JSONL at s3://<active-bucket>/raw/ and otlp/.
-- This file is the canonical column reference; the compaction job reads
-- these envelopes and produces derived Parquet at parquet/v*/events/.
--
-- See docs/trace-schema.md for context. Invariants:
--   - payload_raw contains the post-policy payload that was allowed to leave
--     the machine. It may be redacted by default capture policy.
--   - The envelope shape never changes. payload_schema_ref carries the version
--     of whatever's inside payload_raw.
--   - resource is a JSON map; cohort_id / repo / branch / task_type are
--     duplicated out for cheap predicate-pushdown.

CREATE TABLE IF NOT EXISTS trace_envelope (
    -- Identity
    ingest_id           VARCHAR     PRIMARY KEY,    -- ULID; row identity, dedup key
    ingest_ts           TIMESTAMPTZ NOT NULL,       -- when the collector/sidecar saw it
    ingest_source       VARCHAR     NOT NULL,       -- otlp_grpc | otlp_http | rollout_tail | hook_usersubmit | cc_stop_hook | codex_stop_hook
    source_ts           TIMESTAMPTZ NOT NULL,       -- timestamp inside the event itself

    -- Origin
    host                VARCHAR     NOT NULL,       -- machine identifier (host.name from OTel resource)
    gh_user             VARCHAR,                    -- GitHub login resolved from local `gh` config; cross-machine identity
    harness             VARCHAR     NOT NULL,       -- claude_code | codex_cli
    harness_version     VARCHAR,                    -- e.g. "claude-code-1.0.123", "codex-0.130.0"

    -- Session
    session_id          VARCHAR     NOT NULL,       -- canonical join key across signals
    prior_session_id    VARCHAR,                    -- set on resumed sessions (codex exec resume)
    turn_id             VARCHAR,                    -- per-turn identifier; nullable on session-lifecycle events
    turn_depth          INTEGER,                    -- 0 = root agent, 1 = subagent, 2 = sub-sub, ...

    -- Event
    event_kind          VARCHAR     NOT NULL,       -- harness-native event name, untouched
    signal_type         VARCHAR     NOT NULL,       -- trace | metric | log | transcript | hook

    -- Payload
    payload_raw         VARCHAR     NOT NULL,       -- JSON-serialized native event after local policy/redaction
    payload_schema_ref  VARCHAR     NOT NULL,       -- e.g. "codex.rolloutline.v0.130.0" | "otel.genai.dev" | "cc.hook.usersubmit.v1"
    redaction_policy    VARCHAR,                    -- e.g. basic; empty when redaction disabled
    redaction_applied   BOOLEAN,                    -- true when at least one replacement occurred
    redaction_count     INTEGER,                    -- number of replacements applied before upload

    -- Resource attributes (OTel resource map as JSON)
    resource            VARCHAR,                    -- JSON map of all resource attrs from the OTel resource (or equivalent for non-OTLP)

    -- Frame (denormalized for query efficiency; canonical home is the resource map)
    cohort_id           VARCHAR,                    -- frame tag stamped at session start; resolves to content-capture policy
    repo                VARCHAR,                    -- git repo (typically derived from cwd at session start)
    branch              VARCHAR,                    -- git branch at session start
    task_type           VARCHAR                     -- user-defined: retro | feature | bugfix | exploration | ...
);

-- Partition pruning happens at the Parquet path level (dt=, harness=); these
-- DuckDB indexes are only relevant if a copy is materialized locally.
CREATE INDEX IF NOT EXISTS idx_trace_env_session    ON trace_envelope(session_id);
CREATE INDEX IF NOT EXISTS idx_trace_env_source_ts  ON trace_envelope(source_ts);
CREATE INDEX IF NOT EXISTS idx_trace_env_cohort     ON trace_envelope(cohort_id);
CREATE INDEX IF NOT EXISTS idx_trace_env_harness    ON trace_envelope(harness, source_ts);
