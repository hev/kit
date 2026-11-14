// Extracted production functions with a minimal DOM, following test-stats-path.
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const page=fs.readFileSync(process.argv[2]||'pkg/asks/assets/hosted-template.html','utf8');
// Compile the entire production script as well as the extracted functions.
for(const m of page.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g))new vm.Script(m[1]);
const extract=(a,b)=>page.slice(page.indexOf(a),page.indexOf(b,page.indexOf(a)));
class Element {
 constructor(tag){this.tag=tag;this.children=[];this.attrs={};}
 append(...kids){this.children.push(...kids);}
 setAttribute(k,v){assert.ok(!k.startsWith('on'));this.attrs[k]=String(v);}
 set innerHTML(v){assert.equal(v,'','stored value reached innerHTML');this.children=[];}
 replaceChildren(...kids){this.children=kids;}
 addEventListener(){}
}
const roots=new Map();
const ctx={URL,location:{href:'https://example.test/team/asks/'},document:{createElement:tag=>new Element(tag)},S:{turns:[],agents:[],requests:[],usage:{},agent_usage:{}},$:s=>{if(!roots.has(s))roots.set(s,new Element('div'));return roots.get(s);},usageTotal:u=>Object.values(u).reduce((a,b)=>a+b,0)};
vm.createContext(ctx);
vm.runInContext(extract('const el =','const ago =')+'\n'+extract('function header(){','// ---------------------------------------------------------------- traces view'),ctx);
let failed=false;
// The startup probe deliberately uses stub elements so markup-safety failures
// cannot mask the independent empty-session crash.
try {
 const startup={S:{turns:[],agents:[],requests:[],usage:{},agent_usage:{}},el:()=>({append(){}}),$:()=>({append(){}}),usageTotal:()=>0,safeURL:()=>'',repoLink:()=>''};vm.createContext(startup);
 vm.runInContext(extract('const esc =','const ago =')+'\n'+extract('function header(){','// ---------------------------------------------------------------- traces view'),startup);
 vm.runInContext('header()',startup);console.log('PASS empty-session startup');
}catch(e){failed=true;console.error('FAIL empty-session startup '+e.stack);}

for(const session of [{}, {cost:undefined,repo_url:undefined,model:undefined,start:undefined,wall_ms:undefined}, {repo_url:'x"><img src=x onerror=alert(document.domain)>',project:'<img onerror=bad>',branch:'" onmouseover="bad'}, {repo_url:'javascript:alert(1)'}, {repo_url:'https://example.test/repo',branch:'main'}]){
 ctx.S={turns:[],agents:[],requests:[],usage:{},agent_usage:{},...session};
 try{vm.runInContext('header()',ctx);const walk=n=>{if(typeof n!=='object')return;for(const [k,v] of Object.entries(n.attrs))if(k==='href')assert.match(v,/^https?:/);n.children.forEach(walk);};walk(roots.get('#shead'));console.log('PASS header '+JSON.stringify(session));}catch(e){failed=true;console.error('FAIL header '+JSON.stringify(session)+' '+e.stack);}
}
if(failed)process.exitCode=1;
// Stored free text in snippets, markdown, tool summaries, patches and results
// must remain text even when it resembles markup or attribute syntax.
vm.runInContext(extract('const STOP=','// Who a hit came from')+'\n'+extract('function mdInline(','function renderItems('),ctx);
const payload='<img src=x onerror=alert(1)>"';
ctx.payload=payload;
ctx.agentIndex={};
for(const expression of ['md(payload)','snippetHTML(payload,"img")','renderTool({id:payload,name:payload,summary:payload,result:payload,patch:[{oldStart:payload,oldLines:1,newStart:2,newLines:1,lines:[payload]}],ms:0,start:0,end:1},false)']){
 const root=vm.runInContext(expression,ctx);
 const walk=n=>{if(typeof n!=='object')return;assert.ok(!['script','img'].includes(n.tag));n.children.forEach(walk);};walk(root);
 console.log('PASS stored-content DOM '+expression);
}
