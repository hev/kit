// Copyright the hev kit authors. Licensed under the Apache License, Version 2.0.

import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { test } from 'node:test';

import { context, trace } from '@opentelemetry/api';

import { isConfigured, register, telemetry, tracesURL, validNamespace } from '../src/index.js';

// A fake OTLP/HTTP receiver: records each export request.
async function receiver(status = 200) {
  const requests = [];
  const server = createServer((req, res) => {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      const raw = Buffer.concat(chunks).toString('utf8');
      let body = null;
      try {
        body = JSON.parse(raw);
      } catch {}
      requests.push({ method: req.method, url: req.url, headers: req.headers, body });
      res.writeHead(status, { 'content-type': 'application/json' });
      res.end('{}');
    });
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address();
  return {
    url: `http://127.0.0.1:${port}`,
    requests,
    close: () => new Promise((resolve) => server.close(resolve)),
  };
}

function spansOf(requests) {
  const out = [];
  for (const r of requests) {
    for (const rs of r.body?.resourceSpans ?? []) {
      for (const ss of rs.scopeSpans ?? []) {
        for (const s of ss.spans ?? []) out.push({ span: s, resource: rs.resource });
      }
    }
  }
  return out;
}

function attr(span, key) {
  const kv = (span.attributes ?? []).find((a) => a.key === key);
  if (!kv) return undefined;
  const v = kv.value;
  return v.stringValue ?? v.intValue ?? v.boolValue ?? v.doubleValue;
}

test('tracesURL appends /v1/traces once', () => {
  assert.equal(tracesURL('http://127.0.0.1:4318'), 'http://127.0.0.1:4318/v1/traces');
  assert.equal(tracesURL('http://127.0.0.1:4318/'), 'http://127.0.0.1:4318/v1/traces');
  assert.equal(tracesURL('http://h:1/v1/traces'), 'http://h:1/v1/traces');
});

test('register without an endpoint is a no-op and telemetry is disabled', async () => {
  const shutdown = register({ serviceName: 'ki-agent', endpoint: undefined });
  assert.equal(isConfigured(), false);
  await shutdown();
  const t = telemetry({ userId: 42, chatId: 'c1', turnId: 'ki-run-1', tenant: 'story', functionId: 'ki-agent.run' });
  assert.deepEqual(t, {
    isEnabled: false,
    functionId: 'ki-agent.run',
    metadata: { userId: '42', chatId: 'c1', turnId: 'ki-run-1', tenant: 'story' },
    recordInputs: true,
    recordOutputs: true,
  });
});

test('spans reach the receiver in batches with the key header, nested in one trace', async () => {
  const rx = await receiver();
  const shutdown = register({
    serviceName: 'ki-agent',
    namespace: 'story-test',
    endpoint: rx.url,
    headers: { Authorization: 'Bearer test-key' },
    maxExportBatchSize: 2,
    scheduledDelayMillis: 60_000,
  });
  try {
    assert.equal(isConfigured(), true);
    const t = telemetry({ userId: 7, chatId: 'chat-1', turnId: 'ki-run-9', tenant: 'story', functionId: 'ki-agent.run' });
    assert.equal(t.isEnabled, true);
    assert.equal(t.recordInputs, true);
    assert.equal(t.recordOutputs, true);
    assert.ok(t.tracer, 'telemetry carries the registered tracer');

    // What the AI SDK does with these settings: a run span with the metadata,
    // and step and tool spans opened inside it with startActiveSpan.
    const meta = Object.fromEntries(Object.entries(t.metadata).map(([k, v]) => [`ai.telemetry.metadata.${k}`, v]));
    await t.tracer.startActiveSpan('ai.streamText', { attributes: { ...meta, 'ai.operationId': 'ai.streamText' } }, async (run) => {
      await t.tracer.startActiveSpan('ai.streamText.doStream', { attributes: meta }, async (step) => {
        await new Promise((r) => setTimeout(r, 1));
        step.end();
      });
      await t.tracer.startActiveSpan('ai.toolCall', { attributes: { 'ai.toolCall.name': 'searchStories' } }, async (tool) => {
        tool.end();
      });
      run.end();
    });
    await shutdown();
  } finally {
    await rx.close();
  }

  assert.ok(rx.requests.length >= 2, `three spans at batch size 2 take two requests, got ${rx.requests.length}`);
  for (const r of rx.requests) {
    assert.equal(r.method, 'POST');
    assert.equal(r.url, '/v1/traces');
    assert.equal(r.headers.authorization, 'Bearer test-key');
    assert.match(r.headers['content-type'], /application\/json/);
  }
  const spans = spansOf(rx.requests);
  assert.equal(spans.length, 3);
  const names = spans.map((s) => s.span.name).sort();
  assert.deepEqual(names, ['ai.streamText', 'ai.streamText.doStream', 'ai.toolCall']);
  const traceIds = new Set(spans.map((s) => s.span.traceId));
  assert.equal(traceIds.size, 1, 'step and tool spans share the run span\'s trace');
  const run = spans.find((s) => s.span.name === 'ai.streamText');
  const tool = spans.find((s) => s.span.name === 'ai.toolCall');
  assert.equal(tool.span.parentSpanId, run.span.spanId);
  assert.equal(attr(run.span, 'ai.telemetry.metadata.userId'), '7');
  assert.equal(attr(run.span, 'ai.telemetry.metadata.turnId'), 'ki-run-9');
  const service = run.resource.attributes.find((a) => a.key === 'service.name');
  assert.equal(service.value.stringValue, 'ki-agent');
  for (const s of spans) {
    const ns = s.resource.attributes.find((a) => a.key === 'hev.namespace');
    assert.equal(ns?.value.stringValue, 'story-test', `${s.span.name} carries hev.namespace`);
  }
  assert.equal(isConfigured(), false, 'shutdown clears the configuration');
});

test('export failures never throw into the caller', async () => {
  const errors = [];
  // A receiver that refuses everything, then a port nobody listens on.
  const rx = await receiver(500);
  const closedPort = await receiver();
  await closedPort.close();
  for (const endpoint of [rx.url, closedPort.url]) {
    const shutdown = register({ serviceName: 'ki-agent', namespace: 'story-test', endpoint, exportTimeoutMillis: 2_000, onError: (e) => errors.push(e) });
    const t = telemetry({ userId: 1, turnId: 'ki-run-x', tenant: 'story' });
    const span = t.tracer.startSpan('ai.streamText');
    span.end();
    await assert.doesNotReject(shutdown());
  }
  await rx.close();
  assert.ok(errors.length >= 2, `errors reach onError, got ${errors.length}`);
  for (const e of errors) assert.ok(!String(e?.message ?? e).includes('Bearer'));
});

test('a throwing onError is swallowed too', async () => {
  const closedPort = await receiver();
  await closedPort.close();
  const shutdown = register({
    serviceName: 'ki-agent',
    namespace: 'story-test',
    endpoint: closedPort.url,
    exportTimeoutMillis: 1_000,
    onError: () => {
      throw new Error('handler bug');
    },
  });
  telemetry({ userId: 1 }).tracer.startSpan('s').end();
  await assert.doesNotReject(shutdown());
});

test('the global tracer provider and context manager are the registered ones', async () => {
  const rx = await receiver();
  const shutdown = register({ serviceName: 'ki-agent', namespace: 'story-test', endpoint: rx.url, scheduledDelayMillis: 60_000 });
  try {
    const tracer = trace.getTracer('ai');
    await tracer.startActiveSpan('outer', async (outer) => {
      assert.equal(trace.getSpan(context.active()), outer);
      tracer.startSpan('inner').end();
      outer.end();
    });
    await shutdown();
  } finally {
    await rx.close();
  }
  const spans = spansOf(rx.requests);
  assert.equal(spans.length, 2);
  assert.equal(new Set(spans.map((s) => s.span.traceId)).size, 1);
});

test('validNamespace takes <app>-<env> names and nothing else', () => {
  for (const ok of ['story-prod', 'story-win-oven', 'a', '0', 'x'.repeat(48)]) assert.equal(validNamespace(ok), true, ok);
  for (const bad of [undefined, null, '', '-story', 'Story-prod', 'story_prod', 'story.prod', 'story prod', 'x'.repeat(49), 7]) {
    assert.equal(validNamespace(bad), false, String(bad));
  }
});

test('an endpoint with a missing or invalid namespace is a no-op with one warning', async () => {
  const rx = await receiver();
  const originalWarn = console.warn;
  try {
    for (const namespace of [undefined, '', 'Story_Prod', 'x'.repeat(49)]) {
      const warnings = [];
      console.warn = (...args) => warnings.push(args.join(' '));
      const shutdown = register({
        serviceName: 'ki-agent',
        namespace,
        endpoint: rx.url,
        headers: { Authorization: 'Bearer test-key' },
        scheduledDelayMillis: 10,
      });
      assert.equal(isConfigured(), false, `namespace ${namespace} configures nothing`);
      const t = telemetry({ userId: 1, turnId: 'ki-run-1', tenant: 'story' });
      assert.equal(t.isEnabled, false);
      assert.equal(t.tracer, undefined);
      // The app's own spans on the global API go nowhere near the receiver.
      trace.getTracer('ai').startSpan('ai.streamText').end();
      await assert.doesNotReject(shutdown());
      assert.equal(warnings.length, 1, `one warning for namespace ${namespace}: ${warnings}`);
      assert.match(warnings[0], /namespace/);
      assert.ok(!warnings[0].includes('test-key'), 'the warning never carries the headers');
    }
    await new Promise((r) => setTimeout(r, 50));
    assert.equal(rx.requests.length, 0, 'nothing was exported');
  } finally {
    console.warn = originalWarn;
    await rx.close();
  }
});

test('a missing namespace goes to onError when one is given, and never throws', () => {
  const errors = [];
  let shutdown;
  assert.doesNotThrow(() => {
    shutdown = register({ serviceName: 'ki-agent', endpoint: 'http://127.0.0.1:9', onError: (e) => errors.push(e) });
  });
  assert.equal(errors.length, 1);
  assert.match(errors[0].message, /namespace is required/);
  assert.doesNotThrow(() => register({ endpoint: 'http://127.0.0.1:9', namespace: 'BAD', onError: () => { throw new Error('handler bug'); } }));
  return shutdown();
});
