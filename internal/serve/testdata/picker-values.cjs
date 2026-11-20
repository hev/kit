// Exercise the dashboard's actual request memo without a browser or wall-clock waits.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const template = fs.readFileSync('template.html', 'utf8');
const start = template.indexOf('const valueMemo=');
const end = template.indexOf('\nfunction applyFilters(', start);
assert.ok(start >= 0 && end > start);
let now = 0, calls = 0, archive = [{v:'codex', n:3331}];
let fail = false;
const context = vm.createContext({
  Date: {now: () => now},
  getJSON: async () => {
    calls++;
    if (fail) throw new Error('fixture unavailable');
    return {values: archive.map(row => ({...row})), total: archive.length};
  },
});
vm.runInContext(template.slice(start, end) + '\nglobalThis.load = values;', context);
const any = '/api/values?window=all&facet=harness';
const host = any + '&host=adams-macbook-air';
(async () => {
  const first = context.load(any), concurrent = context.load(any);
  assert.equal(first, concurrent, 'concurrent requests must share a promise');
  await first;
  assert.equal(calls, 1);
  // The server archive is now current after indexing eight rare sessions.
  archive.push({v:'agy', n:8});
  now = 60001;
  assert.ok((await context.load(host)).values.some(row => row.v === 'agy'));
  const reopened = await context.load(any);
  assert.ok(reopened.values.some(row => row.v === 'agy' && row.n === 8),
    'host=Any must show newly indexed agy when the host-scoped picker does');
  // Settled requests must not hide later archive refreshes, even inside a minute.
  archive.push({v:'opencode', n:1});
  assert.ok((await context.load(any)).values.some(row => row.v === 'opencode'));
  fail = true;
  await assert.rejects(context.load(any), /fixture unavailable/);
  fail = false;
  assert.ok((await context.load(any)).values.some(row => row.v === 'agy'));
  console.log('picker memo: rare harness visible with host=Any; concurrent requests deduplicated; failures evicted');
})().catch(error => {console.error(error); process.exitCode = 1;});
