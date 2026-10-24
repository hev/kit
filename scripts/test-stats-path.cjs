// Executes the production loader and renderer with a minimal DOM, no browser.
// Run: node scripts/test-stats-path.cjs
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const html=fs.readFileSync('internal/serve/template.html','utf8');
const extract=(start,end)=>html.slice(html.indexOf(start),html.indexOf(end,html.indexOf(start)));
const reuse=extract('function statsReusable(){','// Held by reference');
const loader=extract('async function refreshArchive(view){','// Filters follow');
const renderer=extract('function statsView(){','// ---------------------------------------------------------------- stats (this trace)');
class Element {
 constructor(){this.children=[];this.style={};this.dataset={};}
 set innerHTML(value){this.html=value;this.children=value.includes('<td>')?Array.from({length:9},()=>new Element()):[];}
 get innerHTML(){return this.html||'';}
 append(...xs){this.children.push(...xs);}
 insertBefore(x){this.children.unshift(x);}
}
const snapshot={generated_at:new Date().toISOString(),days:30,traces:1,project_count:1,spend:4,tokens:100,prompts:2,tools:3,api_hours:1,wall_hours:2,cache_hit:.5,cost_per_prompt:2,projects:[{name:'p',repo:'',n:1,prompts:2,tok:100,cost:4,api:3600000,wall:7200000,series:Array(12).fill(0)}],daily:[{day:'2026-10-24',spend:4,traces:1,models:{m:{tokens:100,traces:1}}}],prompt_grid:Array.from({length:7},()=>Array(24).fill(0)),prompt_coverage:{with:1,total:1},facets:{}};
for(const [alias,days] of [['07d',7],['+7d',7],['030d',30],['+30d',30],['090d',90],['+90d',90]]){
 const init={URLSearchParams,location:{search:'?window='+encodeURIComponent(alias)}};vm.createContext(init);
 vm.runInContext(extract("const WINDOWS=",'let searchTruncated =')+'\nglobalThis.normalized={windowDays,window:params.get(\'window\')};',init);
 assert.equal(init.normalized.windowDays,days);assert.equal(init.normalized.window,days+'d');
}
(async()=>{
 let calls=[],charts=[],loading=[],shown=[],fail=false;
 const nodes=new Map(),tiles=[];
 const content=node=>typeof node==='string'?node:(node.innerHTML||'')+(node.textContent||'')+node.children.map(content).join(' ');
 const ctx={URLSearchParams,Intl,Date,AbortController,archiveController:null,params:new URLSearchParams('window=all&poor=false'),currentView:'v-stats',refreshID:0,archivePending:false,DATA:{corpus:[],rates:{}},built:{},searchRows:null,searchTruncated:false,
 hidePreview(){},setLoading(on,err){loading.push({on,err});},syncCards(){},renderFilterBar(){},
 getJSON:async url=>{calls.push(url);if(fail)throw Error('fixture unavailable');return url.startsWith('/api/stats')?structuredClone(snapshot):{sessions:[{id:'s'}],stats:structuredClone(snapshot),facets:{},timing:{queries:3,layer_ms:1}};},
 show(view){shown.push(view);},$(selector){if(!nodes.has(selector))nodes.set(selector,new Element());return nodes.get(selector);},el(tag,attrs,...children){const node=new Element();if(attrs?.html)node.innerHTML=attrs.html;node.append(...children);return node;},tile(...args){tiles.push(args);return new Element();},spark(){return new Element();},esc:String,fmtDate:String,fmtN:String,fmtUsd:String,fmtTok:String,fmtMs:String,timingLabel(){return '';},shortModel:String,
 requestAnimationFrame(fn){fn();},document:{createElementNS(){return new Element();}},columns(svg,series,opts){charts.push({series,opts});},legend(){return new Element();},setFilter(){},saveFilters(){},rangeBar(){},customOpen:false};
 ctx.STATS_REUSE_MS=30000;vm.createContext(ctx);vm.runInContext(extract('const fmtHours =','const fmtUsd =')+'\n'+reuse+loader+'\n'+renderer,ctx);
 await ctx.refreshArchive();
 assert.equal(ctx.statsReusable(),true);ctx.DATA.statsReceivedAt=Date.now()-31000;assert.equal(ctx.statsReusable(),false);ctx.DATA.statsReceivedAt=Date.now();
 assert.equal(calls.length,1);assert.match(calls[0],/^\/api\/stats\?/);assert.match(calls[0],/poor=false/);assert.equal(ctx.DATA.corpus.length,0);assert.equal(ctx.archivePending,false);
 Object.defineProperty(ctx.DATA,'corpus',{get(){throw Error('Stats accessed session corpus');},configurable:true});
 ctx.statsView();assert.equal(charts.length,2);assert.match(charts[0].opts.tip(0,0),/1 traces/);assert.match(charts[1].opts.tip(0),/1 traces/);
 Object.defineProperty(ctx.DATA,'corpus',{value:[],writable:true,configurable:true});
 calls=[];ctx.currentView='v-traces';await ctx.refreshArchive('traces');assert.equal(calls.length,1);assert.match(calls[0],/^\/api\/sessions\?/);assert.match(calls[0],/include_stats=true/);assert.equal(ctx.DATA.stats.traces,1);
 let release, navigationSignal;const ordinary=ctx.getJSON;ctx.getJSON=(url,signal)=>url.startsWith('/api/stats')?new Promise(resolve=>{navigationSignal=signal;release=()=>resolve(structuredClone(snapshot));}):ordinary(url,signal);
 calls=[];ctx.currentView='v-stats';const pending=ctx.refreshArchive();ctx.currentView='v-traces';assert.equal(navigationSignal.aborted,false,'navigation aborted reusable pending response');release();await pending;await Promise.resolve();await Promise.resolve();assert.ok(calls.some(u=>u.startsWith('/api/sessions?')),'tab change stranded missing list');ctx.getJSON=ordinary;
 // Exercise the actual getJSON/fetch boundary, including real AbortSignals.
 vm.runInContext(extract('async function getJSON(url,signal){','async function refreshArchive(view){'),ctx);
 let pendingFetches=[];
 ctx.fetch=(url,{signal})=>new Promise((resolve,reject)=>{
  const entry={url,signal,resolve:data=>resolve({ok:true,json:async()=>data})};pendingFetches.push(entry);
  signal.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError')),{once:true});
 });
 ctx.currentView='v-stats';const obsolete=ctx.refreshArchive();const first=pendingFetches[0];
 ctx.params.set('model','new');const replacement=ctx.refreshArchive();
 assert.equal(first.signal.aborted,true,'obsolete Stats fetch was not aborted');
 await obsolete;assert.equal(ctx.archivePending,true,'obsolete finally cleared replacement pending state');
 pendingFetches[1].resolve({...structuredClone(snapshot),traces:2});await replacement;
 assert.equal(ctx.DATA.stats.traces,2);assert.equal(ctx.archivePending,false);
 assert.equal(loading.some(x=>x.err?.includes('Aborted')),false,'obsolete abort displayed an error');
 // Supersede a paired list before its headers arrive, not only its later search.
 pendingFetches=[];ctx.currentView='v-traces';const oldList=ctx.refreshArchive('traces');
 ctx.currentView='v-stats';const newStats=ctx.refreshArchive('stats');
 assert.equal(pendingFetches[0].signal.aborted,true,'obsolete paired list was not aborted');
 await oldList;pendingFetches[1].resolve(structuredClone(snapshot));await newStats;
 assert.equal(ctx.DATA.corpus.length,0);
 // A pending paired request is useful across navigation. Keep it alive and
 // reuse the delivered Stats snapshot instead of launching another scan.
 pendingFetches=[];ctx.params.delete('model');ctx.currentView='v-traces';
 const paired=ctx.refreshArchive('traces');ctx.currentView='v-stats';
 assert.equal(pendingFetches[0].signal.aborted,false);
 pendingFetches[0].resolve({sessions:[{id:'s'}],stats:structuredClone(snapshot),facets:{},timing:{queries:3,layer_ms:1}});
 await paired;assert.equal(pendingFetches.length,1);assert.equal(ctx.statsReusable(),true);
 // The list and subsequent search belong to the same refresh. Superseding
 // either phase aborts its fetch; the new Stats request owns the final state.
 pendingFetches=[];ctx.params.set('q','old');ctx.currentView='v-traces';
 const searching=ctx.refreshArchive('traces');
 pendingFetches[0].resolve({sessions:[],stats:structuredClone(snapshot),facets:{},timing:{queries:3,layer_ms:1}});
 await new Promise(setImmediate);
 assert.match(pendingFetches[1].url,/^\/api\/search\?/);assert.equal(pendingFetches[0].signal,pendingFetches[1].signal);
 ctx.params.delete('q');ctx.currentView='v-stats';const newest=ctx.refreshArchive('stats');
 assert.equal(pendingFetches[1].signal.aborted,true);await searching;
 pendingFetches[2].resolve({...structuredClone(snapshot),traces:3});await newest;
 assert.equal(ctx.DATA.stats.traces,3);assert.equal(ctx.searchRows,null);
 // Even a transport that completes despite abort must not overwrite newer data.
 pendingFetches=[];ctx.fetch=(url,{signal})=>new Promise(resolve=>pendingFetches.push({signal,resolve:data=>resolve({ok:true,json:async()=>data})}));
 const late=ctx.refreshArchive('stats');const fresh=ctx.refreshArchive('stats');
 pendingFetches[1].resolve({...structuredClone(snapshot),traces:4});await fresh;
 pendingFetches[0].resolve({...structuredClone(snapshot),traces:99});await late;
 assert.equal(ctx.DATA.stats.traces,4);assert.equal(ctx.archivePending,false);
 ctx.getJSON=ordinary;
 // Use the production status renderer, not a mocked error notification.
 ctx.statusLine=new Element();ctx.DRILL=new Set();ctx.document.body={classList:{toggle(){}}};ctx.document.querySelectorAll=()=>[];
 vm.runInContext(extract('function setLoading(on, err){','// The status line sits'),ctx);
 fail=true;ctx.currentView='v-stats';await ctx.refreshArchive('stats');ctx.statsView();
 assert.equal(ctx.statusLine.textContent,'Could not load stats: fixture unavailable');
 assert.equal(ctx.statusLine.hidden,false);assert.equal(ctx.archivePending,false);
 assert.doesNotMatch(content(ctx.$('#v-stats')),/Loading stats/);
 fail=false;await ctx.refreshArchive('stats');ctx.statsView();
 assert.equal(ctx.statusLine.hidden,true,'successful retry did not clear error');
 assert.equal(ctx.DATA.stats.traces,1);
 ctx.DATA.stats={...structuredClone(snapshot),days:1,traces:0,project_count:0,spend:0,tokens:0,prompts:0,tools:0,api_hours:0,wall_hours:0,cache_hit:0,cost_per_prompt:0,projects:[],daily:[],models:{}};
 tiles.length=0;ctx.statsView();const empty=content(ctx.$('#v-stats'));
 assert.match(empty,/0 traces · last 1 day · 0 projects/);assert.doesNotMatch(empty,/1 days|Loading|NaN|Infinity/);
 assert.equal(tiles.length,6);assert.deepEqual(tiles[3],['API hours','0','of 0 session hours']);
 ctx.DATA.stats={...structuredClone(snapshot),api_hours:3/60,wall_hours:21/60};tiles.length=0;ctx.statsView();
 assert.deepEqual(tiles[3],['API hours','0.05','of 0.35 session hours']);assert.match(content(ctx.$('#v-stats')),/last 30 days/);
 assert.equal(vm.runInContext('fmtHours(1/3600)',ctx),'<0.01');
 // Render the actual empty trace list and its heat drill chip.
 vm.runInContext(extract("let sortKey='end'",'let previewTimer'),ctx);
 ctx.params.set('weekday','4');ctx.params.set('hour','6');ctx.searchRows=null;ctx.DATA.corpus=[];ctx.tracesView();
 assert.match(content(ctx.$('#v-traces')),/Thu · 6:00 ×/);assert.doesNotMatch(content(ctx.$('#v-traces')),/Weekday 4/);
 console.log('PASS: obsolete Stats/list/search fetches abort; paired navigation reuse survives; stale responses cannot overwrite data; cold Stats makes zero sessions requests; compact-only rendering and chart counts; combined list+Stats uses one request; empty/error paths');
})().catch(e=>{console.error(e);process.exit(1);});
