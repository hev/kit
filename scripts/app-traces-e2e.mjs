#!/usr/bin/env node
// Copyright the hev kit authors. Licensed under the Apache License, Version 2.0.
//
// End-to-end check of app capture with no Layer key: `hev collect` on the
// file archive, no Docker, nothing hosted. Runs the same on Linux, macOS and
// Windows (scripts/app-traces-e2e.sh is a wrapper for this).
//
// Needs Go, Node 22+ and npm (and network for npm install). It builds hev,
// starts `hev collect` in a scratch HOME with a scratch archive root and no
// Layer key in its environment, allowing two app namespaces; installs the
// published @hevmind/trace@0.1.0 beside `ai` and `zod`; runs the real Vercel
// AI SDK (mock model) for two users in namespace A, the same user in
// namespace B, and one turn in a namespace the collector does not allow
// (packages/trace-js/e2e/ai-sdk-e2e.mjs asserts the app API: user and
// namespace isolation, conversation fetch, 400/403, the memory board, 401);
// then checks the files: each namespace's turns only in its own archive
// namespace, the refused namespace nowhere, the service key in no log or
// file, and nothing written outside the archive root but kit's config.
//
//   node scripts/app-traces-e2e.mjs          (any OS; PowerShell too)
//   scripts/app-traces-e2e.sh                (the same, from bash)
//
// E2E_PORT picks the port (default 14319), E2E_TRACE_VERSION the published
// @hevmind/trace version (default 0.1.0), E2E_KEEP=1 keeps the scratch
// directory.

import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const win = process.platform === 'win32';
const port = Number(process.env.E2E_PORT || 14319);
const traceVersion = process.env.E2E_TRACE_VERSION || '0.1.0';
const run = String(Math.floor(Date.now() / 1000));
const work = fs.mkdtempSync(path.join(os.tmpdir(), 'hev-app-e2e-'));
const key = 'e2e-' + randomBytes(24).toString('hex');
const nsA = 'story-e2e-a';
const nsB = 'story-e2e-b';
const nsRogue = 'story-e2e-rogue';
const prefix = 'hev-app-traces';
const home = path.join(work, 'home');
const config = path.join(work, 'config', 'config.toml');
const archive = path.join(work, 'archive');
const hev = path.join(work, win ? 'hev.exe' : 'hev');
const logPath = path.join(work, 'collect.log');
let collect;

function cleanup() {
  if (collect && collect.exitCode === null) collect.kill();
  if (process.env.E2E_KEEP) console.log(`kept ${work}`);
  else fs.rmSync(work, { recursive: true, force: true });
}
process.on('exit', cleanup);
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => process.exit(130));

// The collector's environment: a scratch HOME, config and archive root, and
// no Layer key or endpoint under any name kit reads.
function collectEnv(extra = {}) {
  const env = {};
  for (const [k, v] of Object.entries(process.env)) {
    if (/^(HEVLAYER_|LAYER_|HEV_|OP_)/.test(k) || ['TURBOPUFFER_API_KEY', 'TPUF_API_KEY', 'HOME', 'USERPROFILE'].includes(k)) continue;
    env[k] = v;
  }
  return { ...env, HOME: home, USERPROFILE: home, HEV_CONFIG: config, HEV_ARCHIVE_DIR: archive, ...extra };
}

function step(title) {
  console.log(`\n== ${title}`);
}

function sh(cmd, args, opts = {}) {
  const r = spawnSync(cmd, args, { stdio: 'inherit', shell: win && /^(npm|npx)$/.test(cmd), ...opts });
  if (r.status !== 0) throw new Error(`${cmd} ${args.join(' ')} exited ${r.status}`);
  return r;
}

function walk(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, out);
    else out.push(p);
  }
  return out;
}

const rel = (p) => path.relative(work, p).split(path.sep).join('/');
const size = (files) => files.reduce((n, f) => n + fs.statSync(f).size, 0);

step('build hev');
sh('go', ['build', '-o', hev, './cmd/hev'], { cwd: root });

step('hev collect refuses to start without namespaces');
{
  const r = spawnSync(hev, ['collect', '--listen', `127.0.0.1:${port}`], {
    env: collectEnv({ HEV_COLLECT_SERVICE_KEY: key, HEV_COLLECT_NAMESPACES: '' }), encoding: 'utf8',
  });
  const out = (r.stdout + r.stderr).trim();
  assert.notEqual(r.status, 0, 'hev collect started without HEV_COLLECT_NAMESPACES');
  console.log(out.split('\n').at(-1));
  assert.ok(!out.includes(key), 'the refusal printed the key');
}

step(`start hev collect with no Layer key (namespaces ${nsA},${nsB}; archive ${rel(archive)})`);
fs.mkdirSync(home, { recursive: true });
const log = fs.openSync(logPath, 'w');
collect = spawn(hev, ['collect', '--listen', `127.0.0.1:${port}`], {
  env: collectEnv({ HEV_COLLECT_SERVICE_KEY: key, HEV_COLLECT_NAMESPACES: `${nsA},${nsB}` }),
  stdio: ['ignore', log, log],
});
const url = `http://127.0.0.1:${port}`;
let healthy = false;
for (let i = 0; i < 100 && !healthy; i++) {
  if (collect.exitCode !== null) break;
  healthy = await fetch(url + '/healthz').then((r) => r.ok, () => false);
  if (!healthy) await new Promise((r) => setTimeout(r, 200));
}
if (!healthy) {
  console.log(fs.readFileSync(logPath, 'utf8'));
  throw new Error('hev collect did not become healthy');
}
console.log(fs.readFileSync(logPath, 'utf8').trim());
assert.match(fs.readFileSync(logPath, 'utf8'), /\(files .*grep search/, 'hev collect is not on the file archive');

step(`install the published @hevmind/trace@${traceVersion} beside ai and zod`);
const app = path.join(work, 'app');
fs.mkdirSync(app);
fs.copyFileSync(path.join(root, 'packages', 'trace-js', 'e2e', 'ai-sdk-e2e.mjs'), path.join(app, 'ai-sdk-e2e.mjs'));
fs.writeFileSync(path.join(app, 'package.json'), JSON.stringify({ name: 'hev-app-e2e', private: true, type: 'module' }));
sh('npm', ['install', '--silent', '--no-audit', '--no-fund', `@hevmind/trace@${traceVersion}`, 'ai@6', 'zod@4'], { cwd: app });
const installed = JSON.parse(fs.readFileSync(path.join(app, 'node_modules', '@hevmind', 'trace', 'package.json'), 'utf8'));
console.log(`@hevmind/trace ${installed.version}`);
assert.equal(installed.version, traceVersion);

step('run the AI SDK app');
const t0 = performance.now();
sh(process.execPath, ['ai-sdk-e2e.mjs'], {
  cwd: app,
  env: { ...process.env, HEV_COLLECT_URL: url, HEV_COLLECT_SERVICE_KEY: key, E2E_RUN: run, E2E_NS_A: nsA, E2E_NS_B: nsB, E2E_NS_ROGUE: nsRogue },
});
console.log(`app run took ${Math.round(performance.now() - t0)} ms`);

step('search latency over HTTP (20 calls each)');
for (const [ns, owner, q] of [[nsA, '7001', 'heist Lisbon tram'], [nsA, '7002', 'noir Chicago'], [nsB, '7001', 'heist']]) {
  const times = [];
  for (let i = 0; i < 20; i++) {
    const s = performance.now();
    const r = await fetch(url + '/v1/app/traces/search', {
      method: 'POST', headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ namespace: ns, owner, tenant: 'story', query: q }),
    });
    assert.equal(r.status, 200);
    await r.text();
    times.push(performance.now() - s);
  }
  times.sort((a, b) => a - b);
  console.log(`${ns} owner ${owner} "${q}": median ${times[10].toFixed(1)} ms, max ${times[19].toFixed(1)} ms`);
}

collect.kill('SIGTERM');
await new Promise((r) => (collect.exitCode !== null ? r() : collect.once('exit', r)));
fs.closeSync(log);

step('archive files');
const files = walk(work).filter((f) => !f.startsWith(app + path.sep) && f !== hev);
for (const f of walk(archive)) console.log(`${rel(f)}  ${fs.statSync(f).size} B`);
for (const ns of [nsA, nsB]) {
  const dir = path.join(archive, `${prefix}-${ns}`);
  const chunks = walk(path.join(dir, 'chunks'));
  assert.ok(chunks.length > 0, `${ns}: no turn files`);
  for (const f of chunks) {
    for (const line of fs.readFileSync(f, 'utf8').trim().split('\n')) {
      const row = JSON.parse(line);
      assert.equal(row.namespace, ns, `${rel(f)} holds a ${row.namespace} turn`);
      assert.ok(!line.includes('rogue'), `${rel(f)} holds the refused turn`);
    }
  }
  console.log(`${prefix}-${ns}: ${chunks.length} owner file(s), ${size(walk(dir))} B in all`);
}
assert.ok(!fs.existsSync(path.join(archive, `${prefix}-${nsRogue}`)), 'the refused namespace has an archive directory');
for (const f of walk(archive)) {
  const text = fs.readFileSync(f, 'utf8');
  assert.ok(!text.includes(`a-rogue-${run}`), `${rel(f)} holds the refused turn`);
}
console.log('refused namespace: written nowhere');
const envelopes = path.join(archive, 'hev-app-envelopes');
assert.deepEqual(fs.readdirSync(envelopes).sort(), [`ns=${nsA}`, `ns=${nsB}`], 'envelopes outside the allowed namespaces');
for (const ns of [nsA, nsB]) {
  const n = walk(path.join(envelopes, `ns=${ns}`)).reduce((c, f) => c + fs.readFileSync(f, 'utf8').trim().split('\n').length, 0);
  console.log(`${ns} span envelopes: ${n}`);
}
const outside = files.filter((f) => !f.startsWith(archive + path.sep) && f !== logPath && !f.startsWith(config));
assert.deepEqual(outside.map(rel), [], 'written outside the archive root');
console.log(`nothing outside ${rel(archive)} but ${rel(config)} (the redaction salt) and the collector log`);

step('collector log');
const logText = fs.readFileSync(logPath, 'utf8');
const requests = logText.split('\n').filter((l) => l.includes('msg=request'));
console.log(logText.split('\n').filter((l) => l && !l.includes('msg=request')).join('\n'));
console.log(`(${requests.length} request lines; status counts ${JSON.stringify(requests.reduce((m, l) => { const c = (l.match(/status=(\d+)/) || [])[1]; m[c] = (m[c] || 0) + 1; return m; }, {}))})`);
assert.equal((logText.match(/refused spans/g) || []).length, 2, `want one refusal line each for ${nsRogue} and the missing namespace`);
for (const f of [logPath, ...walk(archive)]) assert.ok(!fs.readFileSync(f, 'utf8').includes(key), `the service key reached ${rel(f)}`);
console.log('service key absent from the log and the archive');
console.log('\nE2E PASS');
