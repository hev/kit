# @hevmind/trace

Send a host app's [Vercel AI SDK](https://ai-sdk.dev) traces to
[`hev collect`](https://hevkit.com/docs/app-traces) over
OTLP/HTTP. Apache-2.0. ESM, Node 22 or later.

```ts
import { register, telemetry } from '@hevmind/trace';

const shutdown = register({
  serviceName: 'ki-agent',
  namespace: 'story-prod',               // <app>-<env>; must be on the collector's allowlist
  endpoint: process.env.HEV_COLLECT_URL, // e.g. http://127.0.0.1:4318
  headers: { Authorization: `Bearer ${process.env.HEV_COLLECT_SERVICE_KEY}` },
});

const result = streamText({
  model,
  messages,
  experimental_telemetry: telemetry({ userId, chatId, turnId: `ki-run-${runId}`, tenant: 'story', functionId: 'ki-agent.run' }),
});

process.on('SIGTERM', async () => {
  await shutdown(); // flushes pending spans
  process.exit(0);
});
```

- `register` returns a no-op when `endpoint` is unset, so the same code runs
  with capture off.
- `namespace` (required with an endpoint) is the app's environment, e.g.
  `story-prod`, `story-staging`, matching `^[a-z0-9][a-z0-9-]{0,47}$`. It goes
  on every span's resource as `hev.namespace`; `hev collect` refuses spans
  whose namespace is missing or not on its allowlist. With an endpoint but a
  missing or invalid namespace, `register` warns once and is a no-op.
- Spans are exported in batches by OpenTelemetry's batch span processor. An
  export failure goes to `onError` (default: one `console.warn` line, never the
  headers) and is never thrown into your code.
- `telemetry` returns the AI SDK's `experimental_telemetry` settings:
  `{ isEnabled, functionId, metadata, recordInputs: true, recordOutputs: true }`,
  with `isEnabled` false when `register` wasn't configured. When it is, the
  settings also carry `tracer`, the registered tracer, so spans reach
  `hev collect` even if the app has another global OpenTelemetry setup.
  `userId` is sent as a string.
- `register` installs an async context manager (so a run's step and tool
  spans share its trace) and the global tracer provider, each only if the app
  has not installed one. Pass `global: false` to leave the global provider
  alone.

The service key lets its holder assert any user's identity to `hev collect`.
Keep it on the server: never ship it to a browser or put it in a model's
context.

Options: `serviceName`, `namespace`, `endpoint`, `headers`, `resourceAttributes`, `global`,
`maxExportBatchSize` (256), `scheduledDelayMillis` (2000),
`exportTimeoutMillis` (10000), `maxQueueSize` (4096), `onError`.

## Development

```sh
npm install
npm test        # runs against a local fake OTLP receiver
npm pack        # hevmind-trace-<version>.tgz
```

Published to npm as `@hevmind/trace` (public scope access, see
`publishConfig`).
