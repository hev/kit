// Copyright the hev kit authors. Licensed under the Apache License, Version 2.0.
//
// End-to-end check of app capture: the real Vercel AI SDK (with a mock
// model, so no provider key) runs turns with @hevmind/trace's telemetry,
// exporting to a running `hev collect`; then the app API is asked what an
// agent tool would ask. Each namespace's capture runs in its own process,
// the way each environment's app does: two users in namespace A, the same
// user again in namespace B, and one turn in a namespace the collector does
// not allow. Run by scripts/app-traces-e2e.mjs, which installs this beside
// the published @hevmind/trace, `ai` and `zod`.
//
//   HEV_COLLECT_URL=http://127.0.0.1:4318 HEV_COLLECT_SERVICE_KEY=... \
//   E2E_NS_A=story-e2e-a E2E_NS_B=story-e2e-b E2E_NS_ROGUE=story-e2e-rogue node ai-sdk-e2e.mjs

import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { register, telemetry } from '@hevmind/trace';
import { stepCountIs, streamText, tool } from 'ai';
import { MockLanguageModelV3, convertArrayToReadableStream } from 'ai/test';
import { z } from 'zod';

const url = process.env.HEV_COLLECT_URL;
const key = process.env.HEV_COLLECT_SERVICE_KEY;
assert.ok(url && key, 'HEV_COLLECT_URL and HEV_COLLECT_SERVICE_KEY are required');
const run = process.env.E2E_RUN || String(Date.now());
const nsA = process.env.E2E_NS_A || 'story-e2e-a';
const nsB = process.env.E2E_NS_B || 'story-e2e-b';
const nsRogue = process.env.E2E_NS_ROGUE || 'story-e2e-rogue';

const usage = {
  inputTokens: { total: 10, noCache: 10, cacheRead: 0, cacheWrite: 0 },
  outputTokens: { total: 5, text: 5, reasoning: 0 },
};

// Step 1 calls a tool; step 2 answers.
function model(answer) {
  let step = 0;
  return new MockLanguageModelV3({
    doStream: async () => {
      step++;
      const parts =
        step === 1
          ? [
              { type: 'stream-start', warnings: [] },
              { type: 'tool-call', toolCallId: `call-${run}-${answer.length}`, toolName: 'searchStories', input: '{"q":"setting"}' },
              { type: 'finish', finishReason: { unified: 'tool-calls', raw: 'tool_calls' }, usage },
            ]
          : [
              { type: 'stream-start', warnings: [] },
              { type: 'text-start', id: 't1' },
              { type: 'text-delta', id: 't1', delta: answer },
              { type: 'text-end', id: 't1' },
              { type: 'finish', finishReason: { unified: 'stop', raw: 'stop' }, usage },
            ];
      return { stream: convertArrayToReadableStream(parts) };
    },
  });
}

const searchStories = tool({
  description: 'Search the user\'s stories',
  inputSchema: z.object({ q: z.string() }),
  execute: async ({ q }) => ({ results: [`a story about ${q}`] }),
});

async function turn({ userId, chatId, turnId, prompt, answer }) {
  const result = streamText({
    model: model(answer),
    system: 'You are the Story Agent.',
    messages: [
      { role: 'user', content: 'Hello' },
      { role: 'assistant', content: 'Hi! What are we writing?' },
      { role: 'user', content: prompt },
    ],
    tools: { searchStories },
    stopWhen: stepCountIs(3),
    experimental_telemetry: telemetry({ userId, chatId, turnId, tenant: 'story', functionId: 'ki-agent.run' }),
  });
  return await result.text;
}

// A child process: capture the given turns in one namespace, then exit.
if (process.env.E2E_CAPTURE !== undefined) {
  const namespace = process.env.E2E_CAPTURE || undefined;
  const shutdown = register({ serviceName: 'ki-agent', namespace, endpoint: url, headers: { Authorization: `Bearer ${key}` } });
  for (const t of JSON.parse(process.env.E2E_TURNS)) console.log(`[${namespace ?? 'no namespace'}] answer ${t.turnId}:`, await turn(t));
  await shutdown(); // flush every span to hev collect
  console.log(`[${namespace ?? 'no namespace'}] flushed`);
  process.exit(0);
}

function capture(namespace, turns) {
  const out = execFileSync(process.execPath, [fileURLToPath(import.meta.url)], {
    env: { ...process.env, E2E_CAPTURE: namespace, E2E_TURNS: JSON.stringify(turns) },
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'inherit'],
  });
  process.stdout.write(out);
}

const A = { userId: 7001, chatId: `chat-a-${run}`, turnId: `ki-run-a-${run}`, prompt: 'Draft the opening of a heist story set in Lisbon with a tram chase', answer: 'The tram bell rang as Ines palmed the brass key and jumped for the running board.' };
const B = { userId: 7002, chatId: `chat-b-${run}`, turnId: `ki-run-b-${run}`, prompt: 'Write a noir opening set in rainy Chicago', answer: 'Rain hit the office window like a debt collector who knew my name.' };

// The same user and chat in namespace B: a different turn with overlapping words.
const AinB = { ...A, turnId: `ki-run-a-staging-${run}`, prompt: 'Draft the opening of a heist story set in Lisbon, staging copy', answer: 'Staging: the tram bell rang twice over Alfama.' };
// And in a namespace the collector does not allow.
const AinRogue = { ...A, turnId: `ki-run-a-rogue-${run}`, prompt: 'Draft the opening of a heist story set in Lisbon, rogue copy', answer: 'Rogue: this turn must never be stored.' };

capture(nsA, [A, B]);
capture(nsB, [AinB]);
capture(nsRogue, [AinRogue]);

async function call(method, path, body) {
  const res = await fetch(url + path, {
    method,
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  console.log(`${method} ${path} -> ${res.status} ${text}`);
  return { status: res.status, json: text ? JSON.parse(text) : null };
}

// The turn is indexed when its last export lands; give the store a moment.
async function eventually(fn) {
  for (let i = 0; i < 20; i++) {
    const out = await fn();
    if (out) return out;
    await new Promise((r) => setTimeout(r, 250));
  }
  return fn();
}

// 0. No namespace: register declines (one warning, nothing sent), and a raw
// OTLP export without hev.namespace is refused with 400.
const declined = execFileSync(process.execPath, ['--input-type=module', '-e', `
  import { register, isConfigured } from '@hevmind/trace';
  register({ serviceName: 'ki-agent', endpoint: ${JSON.stringify(url)}, headers: { Authorization: 'Bearer ' + process.env.HEV_COLLECT_SERVICE_KEY } });
  console.log('configured without a namespace:', isConfigured());
`], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] });
process.stdout.write(declined);
assert.match(declined, /configured without a namespace: false/);
const now = `${Date.now()}000000`;
const bare = await call('POST', '/v1/traces', {
  resourceSpans: [{
    resource: { attributes: [{ key: 'service.name', value: { stringValue: 'ki-agent' } }] },
    scopeSpans: [{ scope: { name: 'ai' }, spans: [{
      traceId: '0102030405060708090a0b0c0d0e0f10', spanId: '0102030405060708', name: 'ai.streamText',
      startTimeUnixNano: now, endTimeUnixNano: now,
      attributes: [
        { key: 'ai.telemetry.metadata.userId', value: { stringValue: String(A.userId) } },
        { key: 'ai.telemetry.metadata.turnId', value: { stringValue: `ki-run-bare-${run}` } },
        { key: 'ai.telemetry.metadata.chatId', value: { stringValue: A.chatId } },
        { key: 'ai.telemetry.metadata.tenant', value: { stringValue: 'story' } },
      ],
    }] }],
  }],
});
assert.equal(bare.status, 400, 'a span without a namespace is refused');
assert.equal(bare.json.code, 3, 'the refusal is INVALID_ARGUMENT');

const search = (ns, owner, query) => call('POST', '/v1/app/traces/search', { namespace: ns, owner: String(owner), tenant: 'story', query });

// 1. A's search finds A's turn; B's search with A's words does not.
const aHit = await eventually(async () => {
  const r = await search(nsA, A.userId, 'heist Lisbon tram');
  return r.json.hits.find((h) => h.turnId === A.turnId) ? r : null;
});
assert.ok(aHit, "A's search returns A's turn");
const hit = aHit.json.hits.find((h) => h.turnId === A.turnId);
assert.equal(hit.chatId, A.chatId);
assert.equal(hit.prompt, A.prompt);
assert.ok(hit.answerSnippet.startsWith('The tram bell rang'));

const bSearch = await search(nsA, B.userId, 'heist Lisbon tram');
assert.equal(bSearch.status, 200);
assert.ok(bSearch.json.hits.every((h) => h.turnId !== A.turnId && h.chatId !== A.chatId), "B's search never returns A's turn");
assert.ok(bSearch.json.hits.every((h) => h.turnId === B.turnId), 'B sees only B');

const noOwner = await call('POST', '/v1/app/traces/search', { namespace: nsA, tenant: 'story', query: 'heist' });
assert.equal(noOwner.status, 400);

// 1b. Namespaces: the same user's turn in B is found only in B, and A's only
// in A; a search needs a namespace, and only an allowed one.
const bNsHit = await eventually(async () => {
  const r = await search(nsB, A.userId, 'heist Lisbon tram');
  return r.json.hits.find((h) => h.turnId === AinB.turnId) ? r : null;
});
assert.ok(bNsHit, "A's search in namespace B returns A's namespace-B turn");
assert.ok(bNsHit.json.hits.every((h) => h.turnId === AinB.turnId), 'namespace B holds only its own turn');
const aNsAgain = await search(nsA, A.userId, 'heist Lisbon tram staging rogue copy');
assert.ok(aNsAgain.json.hits.every((h) => h.turnId !== AinB.turnId && h.turnId !== AinRogue.turnId), 'namespace A never shows B\'s or the rogue turn');
assert.equal((await call('POST', '/v1/app/traces/search', { owner: String(A.userId), tenant: 'story', query: 'heist' })).status, 400, 'search without a namespace is 400');
assert.equal((await search(nsRogue, A.userId, 'heist')).status, 403, 'search in a not-allowed namespace is 403');
assert.equal((await search('hev-traces', A.userId, 'heist')).status, 403, 'the archive namespace is not reachable');

// 2. A's conversation, in order, with the tool call; B cannot fetch it.
const conv = await call('GET', `/v1/app/traces/conversations/${A.chatId}?namespace=${nsA}&owner=${A.userId}&tenant=story`);
assert.equal(conv.status, 200);
assert.equal(conv.json.chatId, A.chatId);
assert.equal(conv.json.turns.length, 1, 'all spans of the turn collapsed into one turn');
assert.equal(conv.json.turns[0].turnId, A.turnId);
assert.equal(conv.json.turns[0].prompt, A.prompt);
assert.equal(conv.json.turns[0].answer, A.answer);
assert.deepEqual(conv.json.turns[0].tools, [{ name: 'searchStories', ok: true }]);
const convAsB = await call('GET', `/v1/app/traces/conversations/${A.chatId}?namespace=${nsA}&owner=${B.userId}&tenant=story`);
assert.equal(convAsB.status, 404);
// The same chat id in namespace B is B's own conversation.
const convB = await call('GET', `/v1/app/traces/conversations/${A.chatId}?namespace=${nsB}&owner=${A.userId}&tenant=story`);
assert.equal(convB.status, 200);
assert.deepEqual(convB.json.turns.map((t) => t.turnId), [AinB.turnId], 'namespace B fetch has only B\'s turn');
assert.equal((await call('GET', `/v1/app/traces/conversations/${A.chatId}?owner=${A.userId}&tenant=story`)).status, 400, 'fetch without a namespace is 400');
assert.equal((await call('GET', `/v1/app/traces/conversations/${A.chatId}?namespace=${nsRogue}&owner=${A.userId}&tenant=story`)).status, 403, 'fetch in a not-allowed namespace is 403');

// 3. Memory: post to story-memory with job:123, find it by query and by tag.
const posted = await call('POST', '/v1/app/boards/story-memory/posts', {
  author: String(A.userId), authorName: 'Ana', title: `Ana prefers present tense (${run})`,
  body: 'For project 123, Ana wants second-person narration in the present tense.', tags: ['job:123', 'style'],
});
assert.equal(posted.status, 200);
assert.ok(posted.json.id && posted.json.created);
await call('POST', '/v1/app/boards/story-memory/posts', {
  author: String(B.userId), title: `Ben wants noir (${run})`, body: 'For project 456, Ben wants a hard-boiled voice.', tags: ['job:456'],
});
const byQuery = await call('POST', '/v1/app/boards/story-memory/search', { query: 'second-person narration present tense' });
assert.ok(byQuery.json.hits.some((h) => h.id === posted.json.id && h.authorName === 'Ana' && h.tags.includes('job:123')), 'found by query');
const byTag = await call('POST', '/v1/app/boards/story-memory/search', { tags: ['job:123'] });
assert.ok(byTag.json.hits.some((h) => h.id === posted.json.id), 'found by tag');
assert.ok(byTag.json.hits.every((h) => h.tags.includes('job:123')), 'tag filter holds');
const byQueryAndTag = await call('POST', '/v1/app/boards/story-memory/search', { query: 'narration', tags: ['job:123'] });
assert.ok(byQueryAndTag.json.hits.some((h) => h.id === posted.json.id), 'found by query within the tag');
const otherTag = await call('POST', '/v1/app/boards/story-memory/search', { query: 'narration present tense', tags: ['job:456'] });
assert.ok(otherTag.json.hits.every((h) => h.id !== posted.json.id), 'another job tag excludes it');

// 4. 401 without the key, and the key is not echoed.
const unauth = await fetch(url + '/v1/app/traces/search', { method: 'POST', body: '{}' });
const unauthBody = await unauth.text();
console.log(`POST /v1/app/traces/search (no key) -> ${unauth.status} ${unauthBody}`);
assert.equal(unauth.status, 401);
assert.ok(!unauthBody.includes(key));

console.log('E2E OK');
