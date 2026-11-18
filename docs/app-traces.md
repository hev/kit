# App traces and memory: `hev collect`

`hev collect` is app capture for a host app: a service with its own users,
such as an agent built on the Vercel AI SDK. The app exports OpenTelemetry
traces to it, and the app's own agent tools call it to search that user's past
turns and to save and search memories on a board.

It does only that. It runs no transcript tailing, dispatch, peers or
dashboard; coding-agent capture (`hev up`, `hev d`) is unchanged.

Every span and every API call belongs to a **namespace**, the app's
environment (`story-prod`, `story-staging`), so staging, production and each
developer machine never mix. See [Namespaces](#namespaces).

```text
host app ──OTLP/HTTP──▶ hev collect ──redact──▶ envelopes (JSONL)
   │                        │
   │                        └──turn documents──▶ Layer namespace per app namespace (hybrid search)
   └──app API (search, conversation, boards)──▶ hev collect ──▶ Layer
```

## Run it

```sh
export HEV_COLLECT_SERVICE_KEY=$(openssl rand -hex 32)   # one per host app and environment
export HEV_COLLECT_NAMESPACES=story-prod                  # what this key may write and read
hev collect                     # listens on 127.0.0.1:4318
```

| Flag | Environment | Default |
|---|---|---|
| `--listen` | `HEV_COLLECT_LISTEN` | `127.0.0.1:4318` |
| | `HEV_COLLECT_SERVICE_KEY` | required; `hev collect` refuses to start without it |
| `--namespaces` | `HEV_COLLECT_NAMESPACES` | required (comma list); `hev collect` refuses to start without it |
| `--traces-namespace` | `HEV_COLLECT_TRACES_NAMESPACE` | `hev-app-traces`: the prefix of each app namespace's Layer namespace |
| `--boards-namespace` | `HEV_COLLECT_BOARDS_NAMESPACE` | `hev-app-boards` |
| `--envelopes` | `HEV_COLLECT_ENVELOPES_DIR` | `~/.hev/collect/envelopes` |

The Layer target is the usual one: the `[layer]` block of kit's config, or
`HEVLAYER_ENDPOINT`, `HEVLAYER_API_KEY`, `HEVLAYER_STORE` and
`HEVLAYER_EMBED_MODEL`. On v0.7 that is hosted hev layer (the default, store
unset or `hosted`) or a gateway in front of your own Turbopuffer account
(`HEVLAYER_STORE=turbopuffer`, see [hev layer](hevlayer.md)). v0.7 retired the
local Postgres store, so `HEVLAYER_STORE=pgvector` stops `hev collect` at
startup with `unknown layer store`. The app namespaces are their own; nothing
is written to the trace archive's namespace or the team board's.

## Namespaces

A namespace is one environment of one app, named `<app>-<env>` and matching
`^[a-z0-9][a-z0-9-]{0,47}$`. Story uses:

| Namespace | Where |
|---|---|
| `story-prod` | production |
| `story-staging` | staging |
| `story-win-oven` | the Windows demo host |
| `story-laptop` | a developer laptop |

- **The app declares it.** `register({ namespace })` puts it on every span's
  OTel resource as `hev.namespace`. With an endpoint but no valid namespace,
  `register` logs one warning and captures nothing.
- **The collector enforces it.** `--namespaces` is the allowlist for this
  collector's service key. Each resource's spans go to that resource's
  `hev.namespace`. Spans with no namespace, or one not on the allowlist, are
  refused: the export answers 200 with OTLP `partial_success.rejected_spans`
  counting them, the collector logs each refused namespace once (never the
  key), and the spans are written and indexed nowhere. There is no default
  namespace to fall back to.
- **Reads name it too.** Trace search and conversation fetch require
  `namespace` (400 without it, 403 when it is not on the allowlist), and read
  only that namespace.
- **Layer namespaces are derived.** A namespace's turns live in
  `<--traces-namespace>-<namespace>`, e.g. `hev-app-traces-story-prod`. The app
  only ever names the suffix, and only from the allowlist, so it cannot reach
  another Layer namespace such as the coding-agent trace archive.
- **Envelopes are split** under `<envelopes>/ns=<namespace>/` and carry
  `namespace` in `resource`.
- **Run one collector per environment**, each with its own service key and an
  allowlist of that environment's namespace: production's collector runs with
  `--namespaces story-prod` and the prod key, staging's with
  `--namespaces story-staging` and its own key. A leaked staging key then
  cannot read or write production. One collector may list several namespaces
  (a developer box serving `story-laptop` and a scratch namespace), but every
  namespace it lists shares its key.

Memory boards are not namespaced yet: `--boards-namespace` is one Layer
namespace for every board, as before.

## Send traces

Use [`@hevmind/trace`](../packages/trace-js/README.md) (Node 22+, ESM):

```ts
import { register, telemetry } from '@hevmind/trace';

const shutdown = register({
  serviceName: 'ki-agent',
  namespace: process.env.HEV_NAMESPACE,             // story-prod, story-staging, ...
  endpoint: process.env.HEV_COLLECT_URL,            // http://127.0.0.1:4318
  headers: { Authorization: `Bearer ${process.env.HEV_COLLECT_SERVICE_KEY}` },
});

streamText({
  ...,
  experimental_telemetry: telemetry({ userId, chatId, turnId: `ki-run-${runId}`, tenant: 'story', functionId: 'ki-agent.run' }),
});

process.on('SIGTERM', async () => { await shutdown(); process.exit(0); });
```

`register` is a no-op without an endpoint, and a no-op with one warning when
the endpoint is set but `namespace` is missing or invalid. It exports in
batches and never throws into the app. `telemetry` returns the AI SDK's `experimental_telemetry`
settings (`isEnabled`, `functionId`, `metadata`, `recordInputs: true`,
`recordOutputs: true`), plus the registered `tracer` when configured.

Any OTLP/HTTP exporter works too: `POST /v1/traces` with JSON
(`application/json`) or protobuf (`application/x-protobuf`), optionally
gzipped, the same bearer key, and the resource attribute `hev.namespace`.

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
| resource `hev.namespace` | the app namespace; required and allowlisted |

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
   `<envelopes>/ns=<namespace>/dt=YYYY-MM-DD/harness=app_<service>/envelopes.jsonl`:
   `ingest_source = otlp_http`, `harness = app:<service.name>`,
   `signal_type = trace`, `session_id = chatId`, `turn_id`, `event_kind` = the
   span name, `payload_raw` = the redacted span, `redaction_*`, and `resource`
   with the OTel resource plus `namespace`, `end_user`, `tenant` and `chat_id`. See
   [the trace envelope](trace-schema.md).
3. **One turn document per `(namespace, tenant, owner, turnId)`**, in that
   namespace's Layer namespace: `owner`, `tenant`,
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
| `POST /v1/app/traces/search` | `{namespace, owner, tenant, query, since?, limit?}` | `{"hits":[{chatId, turnId, ts, prompt, answerSnippet, score}]}` |
| `GET /v1/app/traces/conversations/{chatId}?namespace=&owner=&tenant=` | | `{"chatId", "turns":[{turnId, ts, prompt, answer, tools}]}` |
| `POST /v1/app/boards/{board}/posts` | `{author, authorName?, title, body, tags[]}` | `{id, created}` |
| `POST /v1/app/boards/{board}/search` | `{query, tags?, limit?}` | `{"hits":[{id, title, body, author, authorName, created, tags, score}]}` |

- Both trace routes: `namespace` is required (400 without it, 403 when it is
  not on this collector's allowlist).
- Trace search: `owner` and `tenant` are required (400 otherwise), and so is
  `query`. Only that owner's turns in that tenant and namespace come back:
  the store query filters on all three, and the server drops anything else
  before answering.
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
  -d '{"namespace":"story-prod","owner":"7001","tenant":"story","query":"heist in Lisbon","limit":5}'
curl -s "localhost:4318/v1/app/traces/conversations/chat-a?namespace=story-prod&owner=7001&tenant=story" -H "Authorization: Bearer $KEY"
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
- Run one `hev collect` key per app and environment; rotate it by restarting
  with a new `HEV_COLLECT_SERVICE_KEY` and redeploying the app.
- The key reaches only the namespaces on its collector's allowlist. The app
  cannot name a Layer namespace directly.
- The listener defaults to loopback. Expose it beyond the host only behind TLS.

`hev collect` never logs headers, query strings or bodies: its request log is
method, path, status and duration.

## Windows (Docker Desktop)

The demo host runs Windows with Docker Desktop. `hev collect` talks to hosted
hev layer (or a Turbopuffer-lane gateway); there is no local store to start on
v0.7.

1. Run `hev collect`, natively or in Docker.

   Native (`hev.exe` from the release zip, or `go build -o hev.exe ./cmd/hev`):

   ```powershell
   $env:HEV_COLLECT_SERVICE_KEY = "<key>"
   $env:HEV_COLLECT_NAMESPACES = "story-win-oven"
   $env:HEVLAYER_API_KEY = "<hev layer key>"
   hev collect --listen 127.0.0.1:4318
   ```

   For your own Turbopuffer account also set `HEVLAYER_ENDPOINT` to its
   gateway and `HEVLAYER_STORE = "turbopuffer"`.

   Envelopes go to `%USERPROFILE%\.hev\collect\envelopes`; the redaction salt
   lives in `%USERPROFILE%\.hev\config.toml`.

   Docker (the kit image's `collect` variant):

   ```powershell
   docker run -d --name hev-collect --restart unless-stopped `
     -p 127.0.0.1:4318:4318 -v hev-collect:/data `
     -e HEV_CONFIG=/data/config.toml -e HEV_COLLECT_ENVELOPES_DIR=/data/envelopes `
     -e HEV_COLLECT_SERVICE_KEY="<key>" -e HEV_COLLECT_NAMESPACES=story-win-oven `
     -e HEVLAYER_API_KEY="<hev layer key>" `
     hevlayer/kit:<version> collect --listen 0.0.0.0:4318
   ```

2. Point the app at it: `HEV_COLLECT_URL=http://127.0.0.1:4318`, the same
   `HEV_COLLECT_SERVICE_KEY`, and `namespace: 'story-win-oven'` in `register`.
   Turns land in the Layer namespace `hev-app-traces-story-win-oven`.
3. Check: `curl http://127.0.0.1:4318/healthz` answers `{"ok":true}`.

## End-to-end check

`scripts/app-traces-e2e.sh` runs the real AI SDK (mock model) for two users
through the packed `@hevmind/trace` into `hev collect` on scratch namespaces.
It checks search isolation between users and between two app namespaces (the
same user in both), the refusal of spans with no or a not-allowed namespace,
the conversation fetch and the `story-memory` board by query and tag; confirms
the key never reached the log; and deletes the scratch namespaces. It refuses
a non-local Layer endpoint.

It was written for the local Postgres store, which v0.7 retired, so on v0.7
it exits with status 77 (skipped) before doing anything. The `internal/collect`
and `internal/board/store` Go tests cover the same behaviour against a fake
gateway. A local e2e against the file-search backend is planned.

## Board tags

App boards keep `tags` as a JSON string and match tags in `hev collect`, so
they work on stores where a namespace with a gateway-embedded text column
does not take an array attribute (the retired local Postgres store answered
422 `max_vector_fields`). The team board is unchanged.
