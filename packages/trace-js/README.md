# @hevkit/trace

Send a host app's [Vercel AI SDK](https://ai-sdk.dev) traces to
[`hev collect`](https://github.com/hev/kit/blob/main/docs/app-traces.md) over
OTLP/HTTP. Apache-2.0. ESM, Node 22 or later.

```ts
import { register, telemetry } from '@hevkit/trace';

const shutdown = register({
  serviceName: 'ki-agent',
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

Options: `serviceName`, `endpoint`, `headers`, `resourceAttributes`, `global`,
`maxExportBatchSize` (256), `scheduledDelayMillis` (2000),
`exportTimeoutMillis` (10000), `maxQueueSize` (4096), `onError`.

## Development

```sh
npm install
npm test        # runs against a local fake OTLP receiver
npm pack        # hevkit-trace-<version>.tgz
```
