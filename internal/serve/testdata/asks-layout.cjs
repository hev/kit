// Real-browser check of the Asks list and trace header at 390px and 1440px,
// light and dark: no sideways scrolling with the filters shown, filters
// narrow the list and are linkable, both labels render. Run by
// TestAsksLayoutInBrowser when PLAYWRIGHT (a playwright module path) is set;
// CHROMIUM optionally names the browser executable, OUT a screenshot dir.
const {chromium}=require(process.env.PLAYWRIGHT);
const fs=require('node:fs'),path=require('node:path');
const BASE=process.env.BASE,T=process.env.TRACE,OUT=process.env.OUT||'';
if(OUT)fs.mkdirSync(OUT,{recursive:true});
(async()=>{
 const b=await chromium.launch(process.env.CHROMIUM?{executablePath:process.env.CHROMIUM}:{});
 let fails=0,passes=0;
 const check=(where,name,ok,detail)=>{if(ok)passes++;else{fails++;console.log('FAIL',where,name,JSON.stringify(detail));}};
 try{
  for(const width of [390,1440])for(const theme of ['light','dark']){
   const p=await b.newPage({viewport:{width,height:900},colorScheme:theme});
   const errors=[];p.on('pageerror',e=>errors.push(String(e)));p.on('console',m=>{if(m.type()==='error')errors.push(m.text());});
   const rows=()=>p.locator('#v-asks tr.row').count();
   const overflow=()=>p.evaluate(()=>{
    const v=document.querySelector('.view.on'),els=[v,...v.querySelectorAll('*')];
    return {doc:document.documentElement.scrollWidth,vw:innerWidth,view:v.scrollWidth>v.clientWidth+1,
     clipped:els.filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+1&&!e.matches('.t,.t *,.q,.who,.h,.wf-row .track,.wf-row .track *,.wfb,.wfb *,.ask-cell a,select,input')).map(e=>({tag:e.tagName,cls:String(e.className),client:e.clientWidth,scroll:e.scrollWidth})),
     outside:[...v.querySelectorAll('.ask-bar *')].filter(e=>{const r=e.getBoundingClientRect();return r.width>0&&(r.right>innerWidth+1||r.left<-1);}).map(e=>e.tagName+'.'+e.className)};
   });
   const at=async(tag,query,want)=>{
    await p.goto(BASE+query+'#asks');await p.locator('#v-asks .ask-bar select').first().waitFor({timeout:10000});await p.locator('#v-asks tr.row,#v-asks .ask-empty').first().waitFor({timeout:10000});
    const o=await overflow();
    check(tag,'no sideways scrolling with filters shown',o.doc<=o.vw&&!o.view&&o.clipped.length===0&&o.outside.length===0,o);
    check(tag,`${want} rows`,(await rows())===want,await rows());
    if(OUT)await p.screenshot({path:path.join(OUT,`${tag}.png`),fullPage:true});
   };
   const w=`${width}-${theme}`;
   await at('list-'+w,'?window=30d',3);
   check('list-'+w,'filters, no order picker',JSON.stringify(await p.locator('#v-asks .ask-bar select').evaluateAll(s=>s.map(x=>x.getAttribute('aria-label'))))==='["user filter","host filter","project filter"]',null);
   check('list-'+w,'asking host → execution hosts',(await p.locator('#v-asks tr.row',{hasText:'Refresh the docs'}).locator('.ask-hosts').innerText()).replace(/\s+/g,' ').trim()==='laptop → mini, oven',await p.locator('#v-asks tr.row',{hasText:'Refresh the docs'}).locator('.ask-hosts').innerText());
   check('list-'+w,'untraced with reason',(await p.locator('#v-asks tr.row',{hasText:'nightly'}).innerText()).includes('untraced: orchestrator nightly-deps launched this run without trace context'),null);
   // Choosing filters in the page narrows the list and writes the URL.
   await p.locator('select[aria-label="user filter"]').selectOption('local:hev');await p.waitForFunction(()=>location.search.includes('ask_user='));
   await p.locator('#v-asks tr.row').first().waitFor();
   await p.locator('select[aria-label="project filter"]').selectOption('kit');await p.waitForFunction(()=>location.search.includes('ask_project=kit'));
   await p.waitForFunction(()=>document.querySelectorAll('#v-asks tr.row').length===1);
   const url=p.url();
   check('pick-'+w,'combined filters in URL',/ask_user=local%3Ahev/.test(url)&&/ask_project=kit/.test(url),url);
   // The link reproduces the list.
   await p.goto(url);await p.locator('#v-asks tr.row').first().waitFor();
   check('pick-'+w,'linked list reproduces',(await rows())===1&&(await p.locator('#v-asks tr.row').innerText()).includes('nightly'),null);
   await at('filtered-'+w,'?window=30d&ask_user=local%3Ahev&ask_host=mini&ask_project=kit',1);
   await at('pretrace-'+w,'?window=30d&ask_pretrace=1',4);
   const legacy=p.locator('#v-asks tr.row',{hasText:'legacy'});
   check('pretrace-'+w,'before tracing label',(await legacy.locator('.pill.hist').innerText())==='before tracing'&&!(await legacy.innerText()).includes('untraced'),await legacy.innerText());
   // The control toggles it off again.
   await p.locator('#v-asks .ask-bar input[type=checkbox]').uncheck();await p.waitForFunction(()=>!location.search.includes('ask_pretrace'));
   await p.waitForFunction(()=>document.querySelectorAll('#v-asks tr.row').length===3);
   check('pretrace-'+w,'control hides runs from before tracing',(await rows())===3,null);
   // The trace header names the asking host apart from the execution hosts.
   await p.goto(BASE+'?window=30d#ask/'+T);await p.locator('.ask-head .ask-hosts').waitFor({timeout:10000});
   check('trace-'+w,'header hosts',(await p.locator('.ask-head .ask-hosts').innerText()).replace(/\s+/g,' ').trim()==='laptop → mini, oven',await p.locator('.ask-head .ask-hosts').innerText());
   const o=await overflow();
   check('trace-'+w,'no sideways scrolling',o.doc<=o.vw,o);
   if(OUT)await p.screenshot({path:path.join(OUT,`trace-${w}.png`)});
   check('errors-'+w,'no console or page errors',errors.length===0,errors);
   await p.close();
  }
 }finally{await b.close();}
 console.log(`${passes} passed, ${fails} failed`);
 process.exitCode=fails?1:0;
})().catch(e=>{console.error(e);process.exitCode=2;});
