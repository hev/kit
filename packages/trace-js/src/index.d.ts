// Copyright the hev kit authors. Licensed under the Apache License, Version 2.0.
// See LICENSE in this package.

import type { Tracer } from '@opentelemetry/api';

export interface RegisterOptions {
  /** Resource service.name; hev collect writes harness = app:<serviceName>. */
  serviceName?: string;
  /** hev collect's base URL (HEV_COLLECT_URL), e.g. http://127.0.0.1:4318. Unset: register is a no-op. */
  endpoint?: string;
  /** Request headers, e.g. { Authorization: `Bearer ${HEV_COLLECT_SERVICE_KEY}` }. */
  headers?: Record<string, string>;
  /** Extra resource attributes. */
  resourceAttributes?: Record<string, string | number | boolean>;
  /** Also set the global tracer provider (default true; a no-op if the app has one). */
  global?: boolean;
  /** Spans per export request (default 256). */
  maxExportBatchSize?: number;
  /** Delay between scheduled exports in ms (default 2000). */
  scheduledDelayMillis?: number;
  /** Timeout for one export in ms (default 10000). */
  exportTimeoutMillis?: number;
  /** Spans buffered before new ones are dropped (default 4096). */
  maxQueueSize?: number;
  /** Receives export and setup errors instead of console.warn. Never rethrown. */
  onError?: (err: unknown) => void;
}

/** Configure export; returns a shutdown that flushes pending spans. Never throws. */
export function register(options?: RegisterOptions): () => Promise<void>;

/** Whether register configured export. */
export function isConfigured(): boolean;

/** `<endpoint>/v1/traces`. */
export function tracesURL(endpoint: string): string;

export interface TelemetryInput {
  /** The app user's id; hev collect's owner. Sent as a string. */
  userId?: string | number;
  chatId?: string;
  /** One per agent run, e.g. `ki-run-<run id>`. */
  turnId?: string;
  tenant?: string;
  functionId?: string;
  /** More ai.telemetry.metadata.* attributes. */
  metadata?: Record<string, string>;
}

export interface Telemetry {
  isEnabled: boolean;
  functionId?: string;
  metadata: Record<string, string>;
  recordInputs: true;
  recordOutputs: true;
  /** register's tracer, when configured. */
  tracer?: Tracer;
}

/** The AI SDK experimental_telemetry settings for one turn. */
export function telemetry(input?: TelemetryInput): Telemetry;
