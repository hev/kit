# App traces and memory: `hev collect`

`hev collect` is app capture for a host app: a service with its own users,
such as an agent built on the Vercel AI SDK. The app exports OpenTelemetry
traces to it, and the app's own agent tools call it to search that user's past
turns and to save and search memories on a board.

It does only that. It runs no transcript tailing, dispatch, peers or
dashboard; coding-agent capture (`hev up`, `hev d`) is unchanged.

```text
host app ──OTLP/HTTP──▶ hev collect ──redact──▶ envelopes (JSONL)
   │                        │
   │                        └──turn documents──▶ Layer namespace (hybrid search)
   └──app API (search, conversation, boards)──▶ hev collect ──▶ Layer
```

## Run it

```sh
export HEV_COLLECT_SERVICE_KEY=$(openssl rand -hex 32)   # one per host app
hev collect                     # listens on 127.0.0.1:4318
```

| Flag | Environment | Default |
|---|---|---|
| `--listen` | `HEV_COLLECT_LISTEN` | `127.0.0.1:4318` |
| | `HEV_COLLECT_SERVICE_KEY` | required; `hev collect` refuses to start without it |
| `--traces-namespace` | `HEV_COLLECT_TRACES_NAMESPACE` | `hev-app-traces` |
| `--boards-namespace` | `HEV_COLLECT_BOARDS_NAMESPACE` | `hev-app-boards` |
| `--envelopes` | `HEV_COLLECT_ENVELOPES_DIR` | `~/.hev/collect/envelopes` |

The Layer target is the usual one: the `[layer]` block of kit's config, or
`HEVLAYER_ENDPOINT`, `HEVLAYER_API_KEY`, `HEVLAYER_STORE` and
`HEVLAYER_EMBED_MODEL`. Both stores work: the local Postgres gateway from
`hev up --store pgvector`, and hosted hev layer. The app namespaces are their
own; nothing is written to the trace archive's namespace or the team board's.

## Send traces

Use [`@hevkit/trace`](../packages/trace-js/README.md) (Node 22+, ESM):

```ts
import { register, telemetry } from '@hevkit/trace';

const shutdown = register({
  serviceName: 'ki-agent',
  endpoint: process.env.HEV_COLLECT_URL,            // http://127.0.0.1:4318
  headers: { Authorization: `Bearer ${process.env.HEV_COLLECT_SERVICE_KEY}` },
});

streamText({
  ...,
  experimental_telemetry: telemetry({ userId, chatId, turnId: `ki-run-${runId}`, tenant: 'story', functionId: 'ki-agent.run' }),
});

process.on('SIGTERM', async () => { await shutdown(); process.exit(0); });
```

`register` is a no-op without an endpoint, exports in batches, and never
throws into the app. `telemetry` returns the AI SDK's `experimental_telemetry`
settings (`isEnabled`, `functionId`, `metadata`, `recordInputs: true`,
`recordOutputs: true`), plus the registered `tracer` when configured.

Any OTLP/HTTP exporter works too: `POST /v1/traces` with JSON
(`application/json`) or protobuf (`application/x-protobuf`), optionally
gzipped, and the same bearer key.

### What `hev collect` reads

The AI SDK's spans (`ai.streamText`, `ai.streamText.doStream`, `ai.toolCall`,
and the `generateText`/`*Object` equivalents) with these attributes:

| Attribute | Meaning |
|---|---|
| `ai.telemetry.metadata.userId` | the turn's owner (a string) |
| `ai.telemetry.metadata.tenant` | the app's tenant, e.g. `story` |
| `ai.telemetry.metadata.chatId` | the conversation |
| `ai.telemetry.metadata.turnId` | one per agent run, e.g. `ki-run-<run id>` |
| resource `service.name` | the app; the envelope's harness is `app:<service.name>` |

Tool-call spans carry no metadata in the AI SDK; they join their turn by trace
id. A span whose trace has not named its turn waits up to five minutes, and is
written without a turn if none arrives.

### What it writes

1. **Redaction first.** Every string in every span (attributes, nested values,
   status message, resource) passes kit's basic capture policy (the gitleaks
   rule set, `capture.redact` in kit's config) before anything is written or
   indexed. Prompt and answer are scrubbed again after extraction. Board posts
   pass the same policy.
2. **One envelope per span**, as JSON lines under
   `<envelopes>/dt=YYYY-MM-DD/harness=app_<service>/envelopes.jsonl`:
   `ingest_source = otlp_http`, `harness = app:<service.name>`,
   `signal_type = trace`, `session_id = chatId`, `turn_id`, `event_kind` = the
   span name, `payload_raw` = the redacted span, `redaction_*`, and `resource`
   with the OTel resource plus `end_user`, `tenant` and `chat_id`. See
   [the trace envelope](trace-schema.md).
3. **One turn document per `(tenant, owner, turnId)`**: `owner`, `tenant`,
   `chatId`, `turnId`, `ts` (the run's start), `prompt` (the last user
   message), `answer` (the final assistant text) and `tools` (each name with
   ok/error). Spans of one turn arriving over several exports merge into the
   same document, which is upserted and indexed for hybrid search (the text
   column is prompt and answer).

## The app API

Every route except `/healthz` needs `Authorization: Bearer
<HEV_COLLECT_SERVICE_KEY>`, compared in constant time. A missing or wrong key
is a 401 whose body never includes the key. Errors are
`{"error": "<message>"}`.

| Route | Body or query | Returns |
|---|---|---|
| `GET /healthz` | | `{"ok":true}` (no auth) |
| `POST /v1/app/traces/search` | `{owner, tenant, query, since?, limit?}` | `{"hits":[{chatId, turnId, ts, prompt, answerSnippet, score}]}` |
| `GET /v1/app/traces/conversations/{chatId}?owner=&tenant=` | | `{"chatId", "turns":[{turnId, ts, prompt, answer, tools}]}` |
| `POST /v1/app/boards/{board}/posts` | `{author, authorName?, title, body, tags[]}` | `{id, created}` |
| `POST /v1/app/boards/{board}/search` | `{query, tags?, limit?}` | `{"hits":[{id, title, body, author, authorName, created, tags, score}]}` |

- Trace search: `owner` and `tenant` are required (400 otherwise), and so is
  `query`. Only that owner's turns in that tenant come back: the store query
  filters on both, and the server drops anything else before answering.
  `since` is an RFC 3339 time or a lookback (`36h`, `7d`, `2w`). `limit`
  defaults to 10 and is capped at 20. `score` is higher-is-better.
- Conversation: turns in time order; 404 when the owner has no turns in that
  chat (another owner's chat with the same id is invisible).
- Boards: a board is any slug of lowercase letters, digits, `-` and `_` (the
  memory board is `story-memory`). `id` is a string. Tags are lowercased; use
  `job:<project id>` plus free tags. Board search needs a `query`, `tags`, or
  both; every given tag must be on a hit. With tags and no query, the newest
  matching posts come back. `limit` defaults to 10, capped at 20.

```sh
curl -s localhost:4318/v1/app/traces/search -H "Authorization: Bearer $KEY" \
  -d '{"owner":"7001","tenant":"story","query":"heist in Lisbon","limit":5}'
curl -s "localhost:4318/v1/app/traces/conversations/chat-a?owner=7001&tenant=story" -H "Authorization: Bearer $KEY"
curl -s localhost:4318/v1/app/boards/story-memory/posts -H "Authorization: Bearer $KEY" \
  -d '{"author":"7001","authorName":"Ana","title":"Prefers present tense","body":"...","tags":["job:123"]}'
curl -s localhost:4318/v1/app/boards/story-memory/search -H "Authorization: Bearer $KEY" \
  -d '{"query":"narration tense","tags":["job:123"]}'
```

## Trust model

The service key is **per host app**, and its holder is trusted to assert
`owner` (on trace reads) and `author` (on board posts). `hev collect` does not
know the app's users; it believes the app. So:

- The key lives in the app's server environment only. It never goes to a
  browser, a mobile client, or a model's context (not in a system prompt, not
  in a tool description, not in a tool result).
- The app's agent tools call the API from server code and fill `owner` and
  `tenant` from the authenticated session, never from model output.
- Roles and per-user permissions stay in the host app.
- Run one `hev collect` key per app; rotate it by restarting with a new
  `HEV_COLLECT_SERVICE_KEY` and redeploying the app.
- The listener defaults to loopback. Expose it beyond the host only behind TLS.

`hev collect` never logs headers, query strings or bodies: its request log is
method, path, status and duration.

## Windows (Docker Desktop)

The demo host runs Windows with Docker Desktop.

1. Start the local store: `hev up --store pgvector` (gateway on
   `http://127.0.0.1:8080`).
2. Run `hev collect`, natively or in Docker.

   Native (`hev.exe` from the release zip, or `go build -o hev.exe ./cmd/hev`):

   ```powershell
   $env:HEV_COLLECT_SERVICE_KEY = "<key>"
   $env:HEVLAYER_ENDPOINT = "http://127.0.0.1:8080"
   $env:HEVLAYER_API_KEY = "local"
   $env:HEVLAYER_STORE = "pgvector"
   hev collect --listen 127.0.0.1:4318
   ```

   Envelopes go to `%USERPROFILE%\.hev\collect\envelopes`; the redaction salt
   lives in `%USERPROFILE%\.hev\config.toml`.

   Docker (the kit image's `collect` variant):

   ```powershell
   docker run -d --name hev-collect --restart unless-stopped `
     -p 127.0.0.1:4318:4318 -v hev-collect:/data `
     -e HEV_CONFIG=/data/config.toml -e HEV_COLLECT_ENVELOPES_DIR=/data/envelopes `
     -e HEV_COLLECT_SERVICE_KEY="<key>" `
     -e HEVLAYER_ENDPOINT=http://host.docker.internal:8080 `
     -e HEVLAYER_API_KEY=local -e HEVLAYER_STORE=pgvector `
     hevlayer/kit:<version> collect --listen 0.0.0.0:4318
   ```

3. Point the app at it: `HEV_COLLECT_URL=http://127.0.0.1:4318` and the same
   `HEV_COLLECT_SERVICE_KEY`.
4. Check: `curl http://127.0.0.1:4318/healthz` answers `{"ok":true}`.

## End-to-end check

`scripts/app-traces-e2e.sh` runs the real AI SDK (mock model) for two users
through the packed `@hevkit/trace` into `hev collect` on scratch namespaces
of a local `hev up --store pgvector` stack, checks search isolation, the
conversation fetch and the `story-memory` board by query and tag, confirms
the key never reached the log, and deletes the scratch namespaces. It refuses
a non-local Layer endpoint.

## Local store note

On the local Postgres store (layer-gateway 0.7.3), a namespace with a
gateway-embedded text column does not take an array attribute (422
`max_vector_fields`). App boards therefore keep `tags` as a JSON string and
match tags in `hev collect`; the team board is unchanged.
