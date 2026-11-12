// Render the Asks list, a trace page, its detail panel and a session
// breadcrumb with the page's own functions over a small DOM stand-in, using
// API responses a Go test captured from the fixture. Run by
// TestAsksViewRendersFixture.
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const html=fs.readFileSync(process.env.TEMPLATE,'utf8');
const api=JSON.parse(fs.readFileSync(process.env.API,'utf8'));

class Node{
 constructor(tag){this.tagName=tag;this.children=[];this.attrs={};this.listeners={};this.style={};this.parent=null;this._class='';}
 get className(){return this._class;} set className(v){this._class=v;}
 get classList(){const n=this;const list=()=>n._class.split(/\s+/).filter(Boolean);return {add:c=>{if(!list().includes(c))n._class=(n._class+' '+c).trim();},remove:c=>{n._class=list().filter(x=>x!==c).join(' ');},toggle:(c,on)=>{(on??!list().includes(c))?n.classList.add(c):n.classList.remove(c);},contains:c=>list().includes(c)};}
 set innerHTML(v){this.children=[];if(v)this.children.push(new Text(String(v).replace(/<[^>]+>/g,' ')));}
 get textContent(){return this.children.map(c=>c.textContent).join('');}
 set textContent(v){this.children=[new Text(String(v))];}
 append(...kids){for(let k of kids){if(!(k instanceof Node)&&!(k instanceof Text))k=new Text(String(k));if(k.parent)k.remove();k.parent=this;this.children.push(k);}}
 prepend(...kids){const rest=this.children;this.children=[];this.append(...kids);this.children.push(...rest);}
 replaceChildren(...kids){this.children=[];this.append(...kids);}
 after(n){const p=this.parent,i=p.children.indexOf(this);if(n.parent)n.remove();n.parent=p;p.children.splice(i+1,0,n);}
 remove(){if(this.parent){this.parent.children=this.parent.children.filter(c=>c!==this);this.parent=null;}}
 get firstChild(){return this.children[0];} get lastChild(){return this.children[this.children.length-1];}
 setAttribute(k,v){this.attrs[k]=String(v);if(k==='class')this._class=String(v);}
 getAttribute(k){return this.attrs[k];}
 removeAttribute(k){delete this.attrs[k];}
 addEventListener(e,f){(this.listeners[e]??=[]).push(f);}
 set onclick(f){this.addEventListener('click',f);}
 click(){for(const f of this.listeners.click||[])f({preventDefault(){},stopPropagation(){}});}
 all(pred,out=[]){for(const c of this.children)if(c instanceof Node){if(pred(c))out.push(c);c.all(pred,out);}return out;}
 byClass(c){return this.all(n=>n.classList.contains(c));}
 scrollIntoView(){} get offsetWidth(){return 0;}
}
class Text{constructor(t){this.text=t;this.parent=null;} get textContent(){return this.text;} remove(){Node.prototype.remove.call(this);}}

const views={};for(const id of ['v-asks','v-ask','shead','tabs'])views['#'+id]=new Node('section');
const stubs='function md(s){return String(s);}';
const document={createElement:t=>new Node(t),body:new Node('body'),querySelectorAll:()=>[]};
function lineDef(name){
 const m=html.match(new RegExp('^const '+name+' = .*$','m'));assert(m,name);return m[0];
}
function func(name){
 const start=html.indexOf('function '+name+'(');assert(start>=0,name);
 let depth=0,end=html.indexOf('{',start);
 do{if(html[end]==='{')depth++;if(html[end]==='}')depth--;end++;}while(depth&&end<html.length);
 return html.slice(start,end);
}
const block=html.slice(html.indexOf('// ---------------------------------------------------------------- asks: ask-rooted traces'),html.indexOf('// ---------------------------------------------------------------- nav'));
const ctx=vm.createContext({assert,api,document,views,URLSearchParams,Date,Math,Object,String,Number,JSON,isFinite,Infinity,console,toastLog:[]});
vm.runInContext(`
const $ = s => views[s];
${['el','esc','fmtN','fmtMs','fmtTok','fmtUsd','fmtClock','fmtDate','usageTotal','shortModel'].map(lineDef).join('\n')}
const params=new URLSearchParams('window=30d');
const location={hash:'',search:''};
const built={};let shown=[];let currentView='v-ask';
function jumpTo(){}
function show(v){shown.push(v);}
function toast(t){toastLog.push(t);}
async function getJSON(url){const r=api[url];if(!r)throw Error('no fixture for '+url);return JSON.parse(JSON.stringify(r));}
${block}
${func('header')}
${func('route')}
${stubs}
function timingLabel(){return '';}
`,ctx);

(async()=>{
 await vm.runInContext(`(async()=>{
  // The list: one row per trace, the untraced run labelled, not hidden.
  await asksView();
  const rows=views['#v-asks'].byClass('row');
  assert.equal(rows.length,3,'three traces listed');
  const text=rows.map(r=>r.textContent);
  const multi=text.find(t=>t.includes('Refresh the docs navigation'));
  assert.ok(multi.includes('docs-nav-refresh')&&multi.includes('docs-search-index'),'one trace lists both orders');
  const untraced=rows.find(r=>r.textContent.includes('legacy'));
  assert.ok(untraced.classList.contains('untraced')&&untraced.textContent.includes('untraced'),'untraced run is labelled');
  // Clicking a row opens its trace page.
  untraced.click();
  assert.equal(location.hash,'#ask/run%3Alegacy-cleanup');

  // The trace page: asks, both orders, attempts, validation and sessions.
  askState.trace=api.trace;askState.order='';
  await askView();
  const page=views['#v-ask'];
  const kinds=k=>page.all(n=>n.classList.contains('wf-row')&&n.classList.contains(k));
  assert.equal(kinds('ask').length,4,'four asks');
  const names=kinds('order').map(r=>r.textContent.split(/done|order|continued/)[0].replace('↳ ','').trim());
  assert.deepEqual([...new Set(names)].sort(),['docs-nav-refresh','docs-search-index'],'both orders in one trace');
  // Each order is started once; follow-up asks show it again as a continuation.
  assert.equal(kinds('order').filter(r=>!r.textContent.includes('↳')).length,2);
  assert.equal(kinds('run').length,3,'three implementation attempts');
  assert.equal(kinds('validation').length,1,'one validation run');
  assert.equal(kinds('session').length,4,'four worker sessions');
  const rework=kinds('run').find(r=>r.textContent.includes('attempt 2'));
  assert.ok(rework,'rework attempt shown');
  const order=page.all(n=>n.classList.contains('wf-row')).map(r=>r.classList.contains('ask')?'ask:'+r.textContent.slice(0,12):r.className.replace('wf-row ','').replace(' pick','').replace(' sel',''));
  // By ask: the rework ask groups the attempt it launched.
  const at=order.findIndex(x=>x.startsWith('ask:rework'));
  assert.ok(at>0&&order[at+1].startsWith('order')&&order[at+2]==='run','rework ask holds its attempt: '+order.join(','));
  const open=page.all(n=>n.tagName==='a'&&n.classList.contains('open')&&n.textContent==='open ↗');
  assert.equal(open.length,4);
  assert.ok(open.every(a=>/[?&]session=11111111-[^&]+.*#transcript$/.test(a.attrs.href)),'sessions open the session view');
  // Turns expand under a session.
  const session=kinds('session')[0];
  await session.byClass('caret')[0].listeners.click[0]();
  assert.ok(kinds('turn').length>=1&&kinds('tool').length>=3,'turns and tools expand');
  // Sessions and turns are named by what distinguishes them, never by the
  // dispatch brief every worker gets.
  assert.ok(!page.textContent.includes('Read ../order.md'),'no row is labelled with the dispatch brief');
  const sessionText=kinds('session').map(r=>r.byClass('t')[0].textContent);
  assert.ok(sessionText.includes('Regroup docs sidebar by task'),'summary labels a session: '+sessionText);
  assert.ok(sessionText.includes('docs-search-index · attempt 1'),'order and attempt label a session without a summary: '+sessionText);
  assert.ok(kinds('turn')[0].textContent.includes('turn 1 · dispatch brief'),'first turn named as the brief');
  // Every label carries its full text as a title.
  assert.ok(page.all(n=>n.classList.contains('t')&&n.parent&&n.parent.classList.contains('lab')).every(n=>n.getAttribute('title')),'labels have titles');
  assert.ok(kinds('validation')[0].byClass('t')[0].textContent==='docs-nav-refresh-v1','validation run named in full');
  // The legend explains colours and diamonds.
  const legend=page.byClass('wf-legend')[0];
  assert.ok(legend&&['first ask','follow-up ask','order','attempt','validation','worker session'].every(x=>legend.textContent.includes(x)),'legend present');
  // A follow-up ask shows only the part of the order it caused.
  const widthOf=r=>parseFloat(r.byClass('wfb')[0].attrs.style.match(/width:([0-9.]+)%/)[1]);
  const full=kinds('order').find(r=>r.textContent.startsWith('docs-nav-refresh'));
  const cont=kinds('cont');
  assert.equal(cont.length,2,'rework and validate continue the order');
  for(const c of cont)assert.ok(widthOf(c)<widthOf(full)*0.6,'continued part is shorter than the whole order');

  // Selecting a row opens its detail panel.
  const panel=page.byClass('ask-detail')[0];
  assert.ok(panel.getAttribute('hidden')!==undefined,'panel hidden until a row is selected');
  const rootRow=kinds('ask')[0];rootRow.click();
  assert.ok(panel.getAttribute('hidden')===undefined&&rootRow.classList.contains('sel'),'ask selected');
  assert.ok(panel.textContent.includes('two clicks, and make search index')&&panel.textContent.includes('foh-hev-3')&&panel.textContent.includes('6f1c3c1e-foh0'),'ask text and caller');
  full.click();
  assert.ok(panel.textContent.includes('Intent')&&panel.textContent.includes('Group the guides by task')&&panel.textContent.includes('Existing guide URLs still resolve'),'order intent and acceptance: '+panel.textContent);
  kinds('run').find(r=>r.textContent.includes('attempt 1')&&r.textContent.includes('opus')).click();
  assert.ok(panel.textContent.includes('Regrouped the sidebar')&&panel.textContent.includes('d/docs-nav-refresh')&&panel.textContent.includes('done'),'attempt state, branch and report');
  kinds('run').find(r=>r.textContent.includes('attempt 2')).click();
  assert.ok(panel.textContent.includes('example/docs/pull/41')&&panel.textContent.includes('(rework)'),'attempt 2 PR and its own report');
  kinds('validation')[0].click();
  assert.ok(panel.textContent.includes('Verdict')&&panel.textContent.includes('pass')&&panel.textContent.includes('Every guide page is reachable'),'validation verdict');
  panel.byClass('dh')[0].all(n=>n.tagName==='button')[0].click();
  assert.ok(panel.getAttribute('hidden')!==undefined,'panel closes');

  // The order filter narrows the trace and keeps the root ask for context.
  askState.order='docs-search-index';
  await askView();
  assert.equal(kinds('run').length,1);
  assert.equal(kinds('validation').length,0);
  assert.equal(kinds('session').length,1);
  assert.ok(kinds('ask').some(r=>r.textContent.includes('Refresh the docs navigation')),'root ask kept');
  assert.ok(!kinds('ask').some(r=>r.textContent.includes('sidebar')),'unrelated rework ask hidden');

  // The untraced trace page says so.
  askState.trace='run:legacy-cleanup';askState.order='';
  await askView();
  assert.ok(page.textContent.includes('Untraced: no ask was recorded'),'untraced label');
  assert.equal(kinds('session').length,1);

  // A worker session's header leads back to its trace, ask and order.
  globalThis.S=Object.assign(api.session,{agents:[],usage:{},agent_usage:{}});
  header();
  const crumb=views['#shead'].children[0];
  const links=crumb.all(n=>n.tagName==='a').map(a=>[a.textContent,a.attrs.href]);
  // Every part of the breadcrumb is a link: Asks, the trace, the launching
  // ask, the order and the validation run, then the session list.
  assert.equal(JSON.stringify(links.map(l=>l[0]).slice(0,5).map((x,i)=>i===1?'trace':x)),JSON.stringify(['Asks','trace','validate ask','docs-nav-refresh','docs-nav-refresh-v1']));
  assert.ok(crumb.all(n=>n.tagName==='span'&&!n.classList.contains('pill')).length===0,'no plain-text crumb parts');
  assert.ok(links[0][1].endsWith('#asks'),'asks link');
  assert.ok(links[1][1].endsWith('#ask/'+api.trace),'trace link');
  assert.ok(links[2][1].endsWith('#ask/'+api.trace+'/span/a3a3a3a3a3a3a3a3'),'launching ask link');
  assert.ok(links[3][1].endsWith('#ask/'+api.trace+'/order/docs-nav-refresh'),'order link');
  assert.ok(links[4][1].endsWith('#ask/'+api.trace+'/span/c1c1c1c1c1c1c1c1'),'validation run link');
  assert.ok(links.every(([,h])=>!h.includes('session=')),'links leave the session');
  // Following each link: the route opens the trace, scrolled to and
  // highlighting that row with its detail open, or applies the order filter.
  for(const [i,want] of [[2,'a3a3a3a3a3a3a3a3'],[4,'c1c1c1c1c1c1c1c1']]){
   location.hash=links[i][1].slice(links[i][1].indexOf('#'));currentView='v-asks';shown=[];
   route();
   assert.equal(shown.join(),'ask');
   assert.equal(askState.trace,api.trace);assert.equal(askState.span,want);assert.equal(askState.order,'');
   await askView();
   const hit=page.all(n=>n.classList.contains('wf-row')&&n.getAttribute('data-span')===want);
   assert.equal(hit.length,1,'one row for '+want);
   assert.ok(hit[0].classList.contains('sel')&&hit[0].classList.contains('flash'),'row highlighted');
   assert.equal(page.byClass('ask-detail')[0].getAttribute('hidden'),undefined,'detail open');
  }
  location.hash=links[3][1].slice(links[3][1].indexOf('#'));shown=[];route();
  assert.equal(askState.order,'docs-nav-refresh');assert.equal(askState.span,'');
  await askView();
  assert.equal(kinds('validation').length,1);assert.equal(page.all(n=>n.classList.contains('wf-row')&&n.classList.contains('run')&&n.textContent.includes('codex')).length,0,'order filter applied');
 })()`,ctx);
 console.log('PASS: asks list, by-ask waterfall, labels, legend, detail panel, order filter, untraced label, breadcrumb links');
})().catch(e=>{console.error(e);process.exit(1);});
