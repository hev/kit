// Copyright the hev kit authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// @hevmind/trace sends a host app's Vercel AI SDK traces to `hev collect`.
//
//   const shutdown = register({ serviceName: 'ki-agent', namespace: 'story-prod',
//                               endpoint: process.env.HEV_COLLECT_URL,
//                               headers: { Authorization: `Bearer ${key}` } });
//   streamText({ ..., experimental_telemetry: telemetry({ userId, chatId, turnId, tenant: 'story' }) });
//   await shutdown();
//
// It is stock OpenTelemetry: a tracer provider with a batch span processor and
// the OTLP/HTTP exporter. Nothing it does throws into the caller.

import { context, trace } from '@opentelemetry/api';
import { AsyncLocalStorageContextManager } from '@opentelemetry/context-async-hooks';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { BasicTracerProvider, BatchSpanProcessor } from '@opentelemetry/sdk-trace-base';

const TRACER_NAME = 'ai';

/** The resource attribute hev collect routes a span's namespace by. */
export const NAMESPACE_ATTRIBUTE = 'hev.namespace';

const NAMESPACE_RE = /^[a-z0-9][a-z0-9-]{0,47}$/;

/** Whether a namespace is well formed: `^[a-z0-9][a-z0-9-]{0,47}$`, e.g. `story-prod`. */
export function validNamespace(namespace) {
  return typeof namespace === 'string' && NAMESPACE_RE.test(namespace);
}

/** @type {{ provider: BasicTracerProvider, tracer: import('@opentelemetry/api').Tracer, global: boolean } | null} */
let active = null;

const noop = async () => {};

function warn(onError, err) {
  try {
    if (onError) {
      onError(err);
      return;
    }
    // One line, never the headers: they carry the service key.
    console.warn(`@hevmind/trace: ${err && err.message ? err.message : String(err)}`);
  } catch {
    // A failing error handler is not the caller's problem either.
  }
}

/** The OTLP traces URL for an endpoint: `<endpoint>/v1/traces` unless it names that path. */
export function tracesURL(endpoint) {
  const base = String(endpoint).replace(/\/+$/, '');
  return base.endsWith('/v1/traces') ? base : `${base}/v1/traces`;
}

// safeExporter keeps export failures inside the export path: the batch
// processor reports the failed result, and nothing reaches the app.
function safeExporter(inner, onError) {
  let warned = 0;
  return {
    export(spans, done) {
      try {
        inner.export(spans, (result) => {
          if (result && result.code !== 0 && warned < 5) {
            warned++;
            warn(onError, result.error || new Error('trace export failed'));
          }
          try {
            done(result);
          } catch {}
        });
      } catch (err) {
        warn(onError, err);
        try {
          done({ code: 1, error: err });
        } catch {}
      }
    },
    async shutdown() {
      try {
        await inner.shutdown();
      } catch (err) {
        warn(onError, err);
      }
    },
    async forceFlush() {
      try {
        await inner.forceFlush?.();
      } catch (err) {
        warn(onError, err);
      }
    },
  };
}

/**
 * Configure trace export to hev collect. Returns an async shutdown that flushes
 * every pending span; call it on SIGTERM. With no endpoint it configures
 * nothing and returns a no-op; with an endpoint but no valid namespace it
 * warns once and does the same.
 *
 * @param {import('./index.d.ts').RegisterOptions} [options]
 * @returns {() => Promise<void>}
 */
export function register(options = {}) {
  const { serviceName, namespace, endpoint, headers, onError } = options;
  if (!endpoint) return noop;
  if (!validNamespace(namespace)) {
    // hev collect refuses spans without an allowed namespace, so exporting
    // them would only fail later. Say so once and capture nothing.
    warn(
      onError,
      new Error(
        namespace === undefined || namespace === null || namespace === ''
          ? 'register: namespace is required (e.g. story-prod); trace export is off'
          : `register: namespace ${JSON.stringify(String(namespace).slice(0, 64))} must match ^[a-z0-9][a-z0-9-]{0,47}$; trace export is off`,
      ),
    );
    return noop;
  }
  if (active) {
    warn(onError, new Error('register called twice; keeping the first configuration'));
    return shutdownActive(onError);
  }
  try {
    const exporter = new OTLPTraceExporter({
      url: tracesURL(endpoint),
      headers: { ...(headers || {}) },
      timeoutMillis: options.exportTimeoutMillis ?? 10_000,
    });
    const processor = new BatchSpanProcessor(safeExporter(exporter, onError), {
      maxExportBatchSize: options.maxExportBatchSize ?? 256,
      scheduledDelayMillis: options.scheduledDelayMillis ?? 2_000,
      exportTimeoutMillis: options.exportTimeoutMillis ?? 10_000,
      maxQueueSize: options.maxQueueSize ?? 4_096,
    });
    const provider = new BasicTracerProvider({
      resource: resourceFromAttributes({
        ...(options.resourceAttributes || {}),
        'service.name': serviceName || 'unknown_service',
        [NAMESPACE_ATTRIBUTE]: namespace,
      }),
      spanProcessors: [processor],
    });
    // The AI SDK nests its spans with the active context, so a run's step and
    // tool spans share its trace only under an async context manager. These
    // two registrations are no-ops when the app already has its own.
    context.setGlobalContextManager(new AsyncLocalStorageContextManager().enable());
    const global = options.global !== false && trace.setGlobalTracerProvider(provider);
    active = { provider, tracer: provider.getTracer(TRACER_NAME), global };
    return shutdownActive(onError);
  } catch (err) {
    warn(onError, err);
    active = null;
    return noop;
  }
}

function shutdownActive(onError) {
  const mine = active;
  return async () => {
    if (!mine) return;
    try {
      await mine.provider.forceFlush();
      await mine.provider.shutdown();
    } catch (err) {
      warn(onError, err);
    } finally {
      // Release the global provider this registration set, so a later
      // register (a test, a hot reload) can set its own.
      if (mine.global) trace.disable();
      if (active === mine) active = null;
    }
  };
}

/** Whether register configured export. */
export function isConfigured() {
  return active !== null;
}

/**
 * The AI SDK's experimental_telemetry settings for one turn. isEnabled is
 * false when register wasn't configured. The tracer is the one register made,
 * so spans reach hev collect even when the app's global OpenTelemetry API is
 * another copy or another provider.
 *
 * @param {import('./index.d.ts').TelemetryInput} input
 * @returns {import('./index.d.ts').Telemetry}
 */
export function telemetry(input = {}) {
  const { userId, chatId, turnId, tenant, functionId, metadata } = input;
  const meta = { ...(metadata || {}) };
  const put = (key, value) => {
    if (value !== undefined && value !== null && value !== '') meta[key] = String(value);
  };
  put('userId', userId);
  put('chatId', chatId);
  put('turnId', turnId);
  put('tenant', tenant);
  const out = {
    isEnabled: active !== null,
    functionId,
    metadata: meta,
    recordInputs: true,
    recordOutputs: true,
  };
  if (active) out.tracer = active.tracer;
  return out;
}
