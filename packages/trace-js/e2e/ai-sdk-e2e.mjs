// Copyright the hev kit authors. Licensed under the Apache License, Version 2.0.
//
// End-to-end check of app capture: the real Vercel AI SDK (with a mock
// model, so no provider key) runs two users' turns with @hevkit/trace's
// telemetry, exporting to a running `hev collect`; then the app API is asked
// what an agent tool would ask. Run by scripts/app-traces-e2e.sh, which
// installs this beside the packed @hevkit/trace, `ai` and `zod`.
//
//   HEV_COLLECT_URL=http://127.0.0.1:4318 HEV_COLLECT_SERVICE_KEY=... node ai-sdk-e2e.mjs

import assert from 'node:assert/strict';

import { register, telemetry } from '@hevkit/trace';
import { stepCountIs, streamText, tool } from 'ai';
import { MockLanguageModelV3, convertArrayToReadableStream } from 'ai/test';
import { z } from 'zod';

const url = process.env.HEV_COLLECT_URL;
const key = process.env.HEV_COLLECT_SERVICE_KEY;
assert.ok(url && key, 'HEV_COLLECT_URL and HEV_COLLECT_SERVICE_KEY are required');
const run = process.env.E2E_RUN || String(Date.now());

const shutdown = register({ serviceName: 'ki-agent', endpoint: url, headers: { Authorization: `Bearer ${key}` } });

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

const A = { userId: 7001, chatId: `chat-a-${run}`, turnId: `ki-run-a-${run}`, prompt: 'Draft the opening of a heist story set in Lisbon with a tram chase', answer: 'The tram bell rang as Ines palmed the brass key and jumped for the running board.' };
const B = { userId: 7002, chatId: `chat-b-${run}`, turnId: `ki-run-b-${run}`, prompt: 'Write a noir opening set in rainy Chicago', answer: 'Rain hit the office window like a debt collector who knew my name.' };

console.log('answer A:', await turn(A));
console.log('answer B:', await turn(B));
await shutdown(); // flush every span to hev collect
console.log('flushed');

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

// 1. A's search finds A's turn; B's search with A's words does not.
const aHit = await eventually(async () => {
  const r = await call('POST', '/v1/app/traces/search', { owner: String(A.userId), tenant: 'story', query: 'heist Lisbon tram' });
  return r.json.hits.find((h) => h.turnId === A.turnId) ? r : null;
});
assert.ok(aHit, "A's search returns A's turn");
const hit = aHit.json.hits.find((h) => h.turnId === A.turnId);
assert.equal(hit.chatId, A.chatId);
assert.equal(hit.prompt, A.prompt);
assert.ok(hit.answerSnippet.startsWith('The tram bell rang'));

const bSearch = await call('POST', '/v1/app/traces/search', { owner: String(B.userId), tenant: 'story', query: 'heist Lisbon tram' });
assert.equal(bSearch.status, 200);
assert.ok(bSearch.json.hits.every((h) => h.turnId !== A.turnId && h.chatId !== A.chatId), "B's search never returns A's turn");
assert.ok(bSearch.json.hits.every((h) => h.turnId === B.turnId), 'B sees only B');

const noOwner = await call('POST', '/v1/app/traces/search', { tenant: 'story', query: 'heist' });
assert.equal(noOwner.status, 400);

// 2. A's conversation, in order, with the tool call; B cannot fetch it.
const conv = await call('GET', `/v1/app/traces/conversations/${A.chatId}?owner=${A.userId}&tenant=story`);
assert.equal(conv.status, 200);
assert.equal(conv.json.chatId, A.chatId);
assert.equal(conv.json.turns.length, 1, 'all spans of the turn collapsed into one turn');
assert.equal(conv.json.turns[0].turnId, A.turnId);
assert.equal(conv.json.turns[0].prompt, A.prompt);
assert.equal(conv.json.turns[0].answer, A.answer);
assert.deepEqual(conv.json.turns[0].tools, [{ name: 'searchStories', ok: true }]);
const convAsB = await call('GET', `/v1/app/traces/conversations/${A.chatId}?owner=${B.userId}&tenant=story`);
assert.equal(convAsB.status, 404);

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
