// Executes the production loader and renderer with a minimal DOM, no browser.
// Run: node scripts/test-stats-path.cjs
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const html=fs.readFileSync('internal/serve/template.html','utf8');
const extract=(start,end)=>html.slice(html.indexOf(start),html.indexOf(end,html.indexOf(start)));
const reuse=extract('function statsReusable(){','// Held by reference');
const loader=extract('async function refreshArchive(view){','// Filters follow');
const renderer=extract('function statsView(){','// ---------------------------------------------------------------- stats (this trace)');
class Element {
 constructor(){this.children=[];this.style={};}
 set innerHTML(value){this.html=value;this.children=value.includes('<td>')?Array.from({length:9},()=>new Element()):[];}
 get innerHTML(){return this.html||'';}
 append(...xs){this.children.push(...xs);}
 insertBefore(x){this.children.unshift(x);}
}
const snapshot={generated_at:new Date().toISOString(),days:30,traces:1,project_count:1,spend:4,tokens:100,prompts:2,tools:3,api_hours:1,wall_hours:2,cache_hit:.5,cost_per_prompt:2,projects:[{name:'p',repo:'',n:1,prompts:2,tok:100,cost:4,api:3600000,wall:7200000,series:Array(12).fill(0)}],daily:[{day:'2026-10-24',spend:4,traces:1,models:{m:{tokens:100,traces:1}}}],prompt_grid:Array.from({length:7},()=>Array(24).fill(0)),prompt_coverage:{with:1,total:1},facets:{}};
(async()=>{
 let calls=[],charts=[],loading=[],shown=[],fail=false;
 const ctx={URLSearchParams,Intl,Date,params:new URLSearchParams('window=all&poor=false'),currentView:'v-stats',refreshID:0,archivePending:false,DATA:{corpus:[],rates:{}},built:{},searchRows:null,searchTruncated:false,
 hidePreview(){},setLoading(on,err){loading.push({on,err});},syncCards(){},renderFilterBar(){},
 getJSON:async url=>{calls.push(url);if(fail)throw Error('fixture unavailable');return url.startsWith('/api/stats')?structuredClone(snapshot):{sessions:[{id:'s'}],stats:structuredClone(snapshot),facets:{},timing:{queries:3,layer_ms:1}};},
 show(view){shown.push(view);},$(){return new Element();},el(){return new Element();},tile(){return new Element();},spark(){return new Element();},esc:String,fmtDate:String,fmtN:String,fmtUsd:String,fmtTok:String,fmtMs:String,timingLabel(){return '';},shortModel:String,
 requestAnimationFrame(fn){fn();},document:{createElementNS(){return new Element();}},columns(svg,series,opts){charts.push({series,opts});},legend(){return new Element();},setFilter(){},saveFilters(){},rangeBar(){},customOpen:false};
 ctx.STATS_REUSE_MS=30000;vm.createContext(ctx);vm.runInContext(reuse+loader+'\n'+renderer,ctx);
 await ctx.refreshArchive();
 assert.equal(ctx.statsReusable(),true);ctx.DATA.statsReceivedAt=Date.now()-31000;assert.equal(ctx.statsReusable(),false);ctx.DATA.statsReceivedAt=Date.now();
 assert.equal(calls.length,1);assert.match(calls[0],/^\/api\/stats\?/);assert.match(calls[0],/poor=false/);assert.equal(ctx.DATA.corpus.length,0);assert.equal(ctx.archivePending,false);
 Object.defineProperty(ctx.DATA,'corpus',{get(){throw Error('Stats accessed session corpus');},configurable:true});
 ctx.statsView();assert.equal(charts.length,2);assert.match(charts[0].opts.tip(0,0),/1 traces/);assert.match(charts[1].opts.tip(0),/1 traces/);
 Object.defineProperty(ctx.DATA,'corpus',{value:[],writable:true,configurable:true});
 calls=[];ctx.currentView='v-traces';await ctx.refreshArchive('traces');assert.equal(calls.length,1);assert.match(calls[0],/^\/api\/sessions\?/);assert.match(calls[0],/include_stats=true/);assert.equal(ctx.DATA.stats.traces,1);
 let release;const ordinary=ctx.getJSON;ctx.getJSON=url=>url.startsWith('/api/stats')?new Promise(resolve=>{release=()=>resolve(structuredClone(snapshot));}):ordinary(url);
 calls=[];ctx.currentView='v-stats';const pending=ctx.refreshArchive();ctx.currentView='v-traces';release();await pending;await Promise.resolve();await Promise.resolve();assert.ok(calls.some(u=>u.startsWith('/api/sessions?')),'tab change stranded missing list');ctx.getJSON=ordinary;
 fail=true;ctx.currentView='v-stats';await ctx.refreshArchive('stats');assert.match(loading.at(-1).err,/Could not load stats: fixture unavailable/);assert.equal(ctx.archivePending,false);
 ctx.DATA.stats={...structuredClone(snapshot),traces:0,project_count:0,projects:[],daily:[],models:{}};ctx.statsView();
 console.log('PASS: cold Stats makes zero sessions requests; compact-only rendering and chart counts; combined list+Stats uses one request; empty/error paths');
})().catch(e=>{console.error(e);process.exit(1);});
