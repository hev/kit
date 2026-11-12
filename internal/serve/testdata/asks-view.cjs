// Render the Asks list, a trace page and a session breadcrumb with the page's
// own functions over a small DOM stand-in, using API responses a Go test
// captured from the fixture. Run by TestAsksViewRendersFixture.
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
const location={hash:''};
const built={};let shown=[];
function show(v){shown.push(v);}
function toast(t){toastLog.push(t);}
async function getJSON(url){const r=api[url];if(!r)throw Error('no fixture for '+url);return JSON.parse(JSON.stringify(r));}
${block}
${func('header')}
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
  const names=kinds('order').map(r=>r.textContent.split(/done|order|continues/)[0].replace('↳ ','').trim());
  assert.deepEqual([...new Set(names)].sort(),['docs-nav-refresh','docs-search-index'],'both orders in one trace');
  // Each order is started once; follow-up asks show it again as a continuation.
  assert.equal(kinds('order').filter(r=>!r.textContent.includes('↳')).length,2);
  assert.equal(kinds('run').length,3,'three implementation attempts');
  assert.equal(kinds('validation').length,1,'one validation run');
  assert.equal(kinds('session').length,4,'four worker sessions');
  const rework=kinds('run').find(r=>r.textContent.includes('attempt 2'));
  assert.ok(rework,'rework attempt shown');
  const order=page.all(n=>n.classList.contains('wf-row')).map(r=>r.classList.contains('ask')?'ask:'+r.textContent.slice(0,12):r.className.replace('wf-row ',''));
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
  assert.equal(links[0][0],'Asks');
  assert.ok(links[1][1].endsWith('#ask/'+api.trace),'trace link');
  assert.ok(links.some(([t,h])=>t==='docs-nav-refresh'&&h.endsWith('/order/docs-nav-refresh')),'order link');
  assert.ok(crumb.textContent.includes('validate ask')&&crumb.textContent.includes('docs-nav-refresh-v1'),'launching ask and run');
 })()`,ctx);
 console.log('PASS: asks list, by-ask waterfall, order filter, untraced label, session breadcrumb');
})().catch(e=>{console.error(e);process.exit(1);});
