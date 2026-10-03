const $ = s => document.querySelector(s);
const el = (tag, text) => { const e = document.createElement(tag); if (text !== undefined) e.textContent = text; return e; };
const filters = $('#filters'), message = $('#message');
let query = new URLSearchParams(location.search), generation = 0;
for (const name of ['kind', 'status', 'uncertainty']) if (name !== 'kind') filters.elements[name].value = query.get(name) || '';
async function api(path, options) {
  const res = await fetch(path, options);
  if (!res.ok) throw new Error(res.status === 409 ? 'Review changed elsewhere. Reload history before saving.' : `Request failed (${res.status}).`);
  return res.json();
}
function path(id, action) { return `/api/review/items/${encodeURIComponent(id)}/${action}`; }
async function card(item) {
  const article = el('article'); article.dataset.item = item.id;
  article.append(el('h2', `${item.kind}: ${item.prediction}`), el('p', `${item.synthetic ? 'Synthetic sample · ' : ''}Machine prediction · score ${item.score ?? 'unavailable'} ${item.score_unit || '(units unspecified)'} · ${item.uncertain ? 'uncertain' : 'certain'}`), el('p', `Session ${item.session} · turn ${item.turn} · source ${item.source} · policy ${item.policy} · producer ${item.role || 'unspecified'}/${item.instance || 'unspecified'} · eval ${item.eval_id} · sample ${item.sample_class || 'unspecified'}`));
  article.append(el('p', `Context: ${item.context_state || 'availability unknown'} · ${item.context_explanation || 'Load context to inspect availability.'}`));
  if (item.action_refs?.length || item.error_refs?.length) article.append(el('p', `Action references: ${(item.action_refs || []).join(', ') || 'unavailable'} · Error references: ${(item.error_refs || []).join(', ') || 'unavailable'}`));
  for (const j of item.judgments || []) article.append(el('p', `Prior ${j.actor_type} judgment by ${j.actor}: ${j.verdict}${j.note ? ` — ${j.note}` : ''}`));
  const open = el('button', 'Load context and review history'), panel = el('div'); article.append(open, panel);
  open.onclick = async () => {
    open.disabled = true;
    try {
      const [context, history] = await Promise.all([api(path(item.id, 'context')), api(path(item.id, 'reviews'))]);
      panel.replaceChildren(el('h3', `Context: ${context.state}`), el('p', context.explanation), el('p', `Target ${context.target_ref || 'unavailable'} · Ordering: ${context.ordering || 'unspecified'}`));
      for (const b of context.blocks || []) {panel.append(el('h4', `${b.target ? 'Target · ' : ''}${b.role} · ${b.source} · turn ${b.turn || 'unspecified'} · ${(b.related_to || []).join(', ')}`), el('pre', b.text));}
      const heading = el('h3', 'Verified human review history'), records = el('ol'); panel.append(heading, records);
      for (const r of history || []) records.append(el('li', `Revision ${r.revision} · ${r.actor_type} · ${r.reviewer} · ${r.at}: ${r.verdict}${r.note ? ` — ${r.note}` : ''}`));
      let revision = history?.at(-1)?.revision || 0;
      const form = el('form'), verdict = el('select'), note = el('textarea'), save = el('button', 'Save review'), feedback = el('p'); feedback.setAttribute('role', 'status');
      verdict.setAttribute('aria-label', 'Judgment'); note.setAttribute('aria-label', 'Optional note'); note.maxLength = 8000;
      for (const v of ['correct','incorrect','uncertain']) {const o = el('option', v);o.value = v;verdict.append(o);}
      if (revision) {verdict.value = history.at(-1).verdict;note.value = history.at(-1).note || '';}
      form.append(verdict, note, save, feedback);panel.append(form);
      form.onsubmit = async e => {
        e.preventDefault();save.disabled = true;
        try {
          const r = await api(path(item.id, 'reviews'), {method:'POST',headers:{'Content-Type':'application/json','X-Kit-Review':'1'},body:JSON.stringify({verdict:verdict.value,note:note.value,expected_revision:revision})});
          revision = r.revision;records.append(el('li', `Revision ${r.revision} · ${r.actor_type} · ${r.reviewer} · ${r.at}: ${r.verdict}${r.note ? ` — ${r.note}` : ''}`));feedback.textContent = 'Saved. Reload or apply filters to refresh coverage.';
        } catch (err) {feedback.textContent = err.message;} finally {save.disabled = false;}
      };
      open.textContent = 'Reload context and history';
    } catch (err) {panel.replaceChildren(el('p', err.message));} finally {open.disabled = false;}
  };
  return article;
}
async function load() {
  const current = ++generation;message.textContent = 'Loading…';$('#next').hidden = true;
  history.replaceState(null, '', `/review?${query}`);
  try {
    const {page, kinds} = await api(`/api/review/queue?${query}`);
    if (current !== generation) return;
    filters.elements.kind.replaceChildren(el('option', 'All kinds'));filters.elements.kind.firstChild.value = '';
    for (const k of kinds) {const o = el('option', k);o.value = k;filters.elements.kind.append(o);}filters.elements.kind.value = query.get('kind') || '';
    $('#coverage').textContent = `${page.truncated ? 'INCOMPLETE / TRUNCATED: ' : ''}${page.coverage || 'Coverage not described by provider.'}`;
    $('#totals').replaceChildren();
    for (const t of page.totals || []) {const row = el('tr');for (const key of ['kind','reviewed','remaining','uncertain','required','shortage']) row.append(el('td', key === 'kind' ? `${t.kind} · ${t.sample_class || 'unspecified population'} · ${t.state || 'unavailable'}` : (!t.state || t.state === 'unavailable' ? 'Unavailable' : (t.state === 'lower_bound' && key === 'shortage' ? `Observed ${t[key]}` : `${t.state === 'lower_bound' && key !== 'required' ? '≥ ' : ''}${t[key]}`))));$('#totals').append(row);}
    $('#items').replaceChildren(...await Promise.all((page.items || []).map(card)));
    message.textContent = page.items?.length ? `${page.items.length} labels on this page.` : 'No labels match these filters.';
    $('#next').hidden = !page.next;$('#next').onclick = () => {query.set('cursor', page.next);load();};
  } catch (err) {message.textContent = err.message;$('#items').replaceChildren();}
}
filters.onsubmit = e => {e.preventDefault();query = new URLSearchParams(new FormData(filters));load();};
window.addEventListener('popstate', () => {query = new URLSearchParams(location.search);load();});
load();
