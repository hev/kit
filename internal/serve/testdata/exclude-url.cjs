// Exercise the page's actual state functions without a browser or live archive.
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const html=fs.readFileSync('internal/serve/template.html','utf8');
const names=['saveFilters','valuesURL','applyFilters','setFilter','filterSummary'];
function source(name){
 const start=html.indexOf('function '+name+'(');
 assert(start>=0,name);
 // These functions contain no template literals or braces in strings.
 let depth=0,end=html.indexOf('{',start);
 const body=end;
 do {if(html[end]==='{')depth++;if(html[end]==='}')depth--;end++;}while(depth&&end<html.length);
 assert(end>body,name);
 return html.slice(start,end);
}
const context=vm.createContext({URL,URLSearchParams,assert});
vm.runInContext(`
const params=new URLSearchParams('window=30d&project=alpha');
const location={hash:'#traces'};
let saved='';const history={pushState:(_a,_b,url)=>{saved=url;}};
const RANGES={},tz='UTC';
const facetKey=key=>key.replace(/^exclude_/,'');
const filterView=()=> 'traces',renderChips=()=>{},syncCards=()=>{},refreshArchive=()=>{};
${names.map(source).join('\n')}
setFilter('exclude_project',['hevmind-promo','noise']);
assert.equal(filterSummary('exclude_project'),'hevmind-promo +1');
const reloaded=new URLSearchParams(saved.slice(1).split('#')[0]);
assert.equal(reloaded.getAll('exclude_project').join(','),'hevmind-promo,noise');
const values=new URL(valuesURL('exclude_project'),'http://fixture');
assert.equal(values.searchParams.get('facet'),'project');
assert.equal(values.searchParams.has('project'),false);
assert.equal(values.searchParams.get('exclude_project'),'hevmind-promo');
setFilter('exclude_project',[]);
assert.equal(new URLSearchParams(saved.slice(1).split('#')[0]).has('exclude_project'),false);
assert.equal(params.get('project'),'alpha');
`,context);
console.log('PASS: exclusion URL round-trip, picker scope, summary and removal');
