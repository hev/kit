// The desktop sidebar and narrow-screen drawer share the same navigation.
export function mountWorkspace(tier) {
  const byID = id => document.getElementById(id);
  const collapse = byID('workspace-collapse');
  const drawer = byID('workspace-drawer');
  const dialog = byID('workspace-search-dialog');
  const input = byID('workspace-search-input');
  const results = byID('workspace-results');
  const status = byID('workspace-search-status');

  function setCompact(compact) {
    document.body.classList.toggle('workspace-compact', compact);
    collapse.setAttribute('aria-expanded', String(!compact));
    collapse.textContent = compact ? '»' : '«';
  }

  try { setCompact(localStorage.getItem('kit.sidebar') === 'compact'); } catch {}
  collapse.addEventListener('click', () => {
    const compact = collapse.getAttribute('aria-expanded') === 'true';
    setCompact(compact);
    try { localStorage.setItem('kit.sidebar', compact ? 'compact' : 'expanded'); } catch {}
  });
  drawer.addEventListener('click', () => {
    const open = document.body.classList.toggle('workspace-drawer-open');
    drawer.setAttribute('aria-expanded', String(open));
  });

  let priorFocus;
  function openSearch() {
    if (dialog.open) return;
    priorFocus = document.activeElement;
    dialog.showModal();
    input.focus();
    input.select();
  }
  dialog.addEventListener('close', () => priorFocus?.focus());
  byID('workspace-search').addEventListener('click', openSearch);
  byID('workspace-search-close').addEventListener('click', () => dialog.close());
  document.addEventListener('keydown', event => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      openSearch();
    }
  }, {capture: true});

  const sources = [{
    label: 'Traces', path: '/api/search', query: {window: 'all', top: '8'},
    read: data => (data.sessions || []).map(row => ({
      title: row.summary || row.first_prompt_short || row.id,
      href: '/?' + new URLSearchParams({session: row.id}) + '#transcript'
    }))
  }];
  if (tier !== 'oss') sources.push({
    label: 'Board', path: '/api/board/search', query: {top: '8'},
    read: data => (data || []).map(({thread}) => ({
      title: thread.title, href: '/board#board/' + encodeURIComponent(thread.id)
    }))
  }, ...['Evidence', 'Orders'].map(label => {
    const path = '/' + label.toLowerCase();
    return {label, path: '/api' + path, read: (data, q) => (data.rows || []).map(row => ({
      title: row.title,
      href: path + '?' + new URLSearchParams({q}) + '#' + encodeURIComponent(row.id || '')
    }))};
  }));

  let controller;
  dialog.querySelector('form').addEventListener('submit', async event => {
    event.preventDefault();
    controller?.abort();
    const q = input.value.trim();
    results.replaceChildren();
    if (!q) {
      status.textContent = 'Enter a search term.';
      return;
    }
    const request = new AbortController();
    controller = request;
    status.textContent = 'Searching…';
    const groups = await Promise.all(sources.map(async source => {
      try {
        const query = new URLSearchParams({...source.query, q});
        const response = await fetch(source.path + '?' + query, {signal: request.signal});
        if (!response.ok) throw new Error('unavailable');
        return {label: source.label, rows: source.read(await response.json(), q).slice(0, 8)};
      } catch {
        return {label: source.label, error: true, rows: []};
      }
    }));
    if (request.signal.aborted) return;
    for (const group of groups) {
      const section = document.createElement('section');
      const heading = document.createElement('h2');
      heading.textContent = group.label;
      section.append(heading);
      for (const row of group.rows) {
        const link = document.createElement('a');
        link.href = row.href;
        link.textContent = row.title;
        section.append(link);
      }
      if (!group.rows.length) section.append(group.error ? 'Source unavailable.' : 'No matches.');
      results.append(section);
    }
    status.textContent = '';
  });
}
