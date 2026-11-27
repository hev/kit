// Exercise the production sidebar, keyboard and search module without a browser.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

class Element {
  children = [];
  events = {};
  attrs = {};
  classList = {toggle: (name, force) => {
    const enabled = force ?? !this.attrs[name];
    this.attrs[name] = enabled;
    return enabled;
  }};

  setAttribute(name, value) { this.attrs[name] = value; }
  getAttribute(name) { return this.attrs[name]; }
  addEventListener(name, fn) { this.events[name] = fn; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  showModal() { this.open = true; }
  close() { this.open = false; this.events.close(); }
  focus() { this.focused = true; }
  select() {}
  querySelector() { return this.form ??= new Element(); }
}

(async () => {
  const shell = fs.readFileSync('internal/serve/ui/workspace.mjs', 'utf8');
  const html = fs.readFileSync('internal/serve/template.html', 'utf8');
  const start = html.indexOf('const darkMQ=matchMedia');
  const theme = html.slice(start, html.indexOf('function workspaceView()', start));
  for (const tier of ['oss', 'pro', 'hosted']) {
    const nodes = new Map();
    const stored = new Map();
    const calls = [];
    const document = new Element();
    document.body = new Element();
    document.documentElement = {dataset: {}, removeAttribute(name) {
      delete this.dataset[name.slice('data-'.length)];
    }};
    document.createElement = () => new Element();
    document.getElementById = id => {
      if (!nodes.has(id)) nodes.set(id, new Element());
      return nodes.get(id);
    };
    document.activeElement = new Element();
    const byID = document.getElementById;
    const dialog = byID('workspace-search-dialog');
    const context = vm.createContext({
      document, URLSearchParams, AbortController, tier, $: id => byID(id.slice(1)),
      matchMedia: () => ({matches: false}),
      localStorage: {getItem: key => stored.get(key), setItem: (key, value) => stored.set(key, value)},
      fetch: async input => {
        const url = new URL(input, 'https://fixture.invalid');
        calls.push(url.pathname);
        assert.equal(url.searchParams.get('q'), 'needle');
        const fixtures = {
          '/api/search': {sessions: [{id: 's', summary: 'Trace needle'}]},
          '/api/board/search': [{thread: {id: 1, title: 'Board needle'}}],
          '/api/evidence': {rows: [{id: 'e', title: 'Evidence needle'}]},
          '/api/orders': {rows: [{id: 'o', title: 'Order needle'}]},
        };
        return {ok: true, json: async () => fixtures[url.pathname]};
      },
    });
    vm.runInContext(shell.replace('export function', 'function') + '\nmountWorkspace(tier);\n' + theme, context);
    const collapse = byID('workspace-collapse');
    assert.equal(collapse.attrs['aria-expanded'], 'true');
    collapse.events.click();
    assert.equal(collapse.attrs['aria-expanded'], 'false');
    assert.equal(stored.get('kit.sidebar'), 'compact');
    assert.equal(document.documentElement.dataset.appearance, undefined);
    byID('workspace-theme').onclick();
    assert.equal(stored.get('kit.theme'), 'dark');
    assert.equal(document.documentElement.dataset.appearance, 'dark');
    byID('workspace-theme').onclick();
    assert.equal(stored.get('kit.theme'), 'light');
    assert.equal(document.documentElement.dataset.appearance, undefined);

    for (const modifier of ['ctrlKey', 'metaKey']) {
      let prevented = false;
      document.events.keydown({key: 'K', [modifier]: true, preventDefault: () => {prevented = true;}});
      assert.equal(prevented, true);
      assert.equal(dialog.open, true);
      dialog.close();
      assert.equal(document.activeElement.focused, true);
    }
    byID('workspace-search').events.click();
    byID('workspace-search-input').value = 'needle';
    await dialog.querySelector().events.submit({preventDefault() {}});
    const paths = ['/api/search', '/api/board/search', '/api/evidence', '/api/orders'];
    const labels = ['Traces', 'Board', 'Evidence', 'Orders'];
    assert.deepEqual(calls, tier === 'oss' ? paths.slice(0, 1) : paths);
    const groups = byID('workspace-results').children;
    assert.deepEqual(groups.map(group => group.children[0].textContent), tier === 'oss' ? labels.slice(0, 1) : labels);
    assert.ok(groups.every(group => group.children[1].href && /needle$/.test(group.children[1].textContent)));
  }
  console.log('PASS: sidebar collapse, theme persistence, both keyboard shortcuts, and tier-scoped search fan-out');
})().catch(error => {console.error(error); process.exitCode = 1;});
