// Read-only live-namespace latency checks. Each binary round uses a fresh child
// on an OS-assigned port; URL mode never restarts the deployed service.
// NODE_PATH=<playwright node_modules> HEV_DASHBOARD_BINARY=<binary> node this-file
// HEV_DASHBOARD_URL=<deployment> is observational; failures still exit nonzero.
const {chromium}=require('playwright');
const {spawn,execFileSync}=require('node:child_process');
const {once}=require('node:events');
const fs=require('node:fs');
const assert=require('node:assert/strict');
const dir=process.env.HEV_EVIDENCE_DIR;
if(!dir)throw Error('HEV_EVIDENCE_DIR is required');
fs.mkdirSync(dir,{recursive:true});
const binary=process.env.HEV_DASHBOARD_BINARY;
const report={at:new Date().toISOString(),kind:binary?'isolated branch, live namespace':'current deployed, process temperature uncontrolled',samples:[],failures:[]};
async function start(){
 if(!binary)return {base:process.env.HEV_DASHBOARD_URL};
 const child=spawn(binary,['serve','--port','0'],{stdio:['ignore','pipe','pipe']});
 const base=await new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>reject(Error('child startup timeout')),15000);
  child.stdout.on('data',b=>{const m=String(b).match(/http:\/\/127\.0\.0\.1:\d+/);if(m){clearTimeout(timer);resolve(m[0]);}});
  child.once('error',reject);child.once('exit',c=>reject(Error('child exited '+c)));
 });
 return {base,child};
}
async function stop(child){if(child){const done=once(child,'exit');child.kill('SIGTERM');await done;}}
function check(condition,message){if(!condition)report.failures.push(message);}
(async()=>{
 const browser=await chromium.launch({headless:true});report.browser=await browser.version();
 try{
  for(let round=1;round<=3;round++){
   // Separate API and browser cold starts: curl never warms the browser child.
   let server=await start();
   try{
    for(const temperature of ['cold','warm']){
     const prefix=dir+`/api-${round}-${temperature}`;
     const result=execFileSync('curl',['--fail','--silent','--show-error','--max-time','60','--compressed','-D',prefix+'.headers','-o',prefix+'.json','-w','%{size_download} %{time_total} %{http_code}',server.base+'/api/sessions?window=30d'],{encoding:'utf8'}).trim().split(' ');
     const data=JSON.parse(fs.readFileSync(prefix+'.json'));
     const headers=fs.readFileSync(prefix+'.headers','utf8');
     const sample={kind:'api',round,temperature:binary?temperature:'serial-'+temperature,base:server.base,bytes:Number(result[0]),ms:Number(result[1])*1000,status:Number(result[2]),rows:data.sessions.length,timing:data.timing};report.samples.push(sample);
     check(sample.bytes<2000000&&sample.ms<1500,`API ${round}/${temperature}: ${sample.ms}ms, ${sample.bytes} bytes`);
     assert.match(headers,/content-encoding: gzip/i);assert.match(headers,/vary: Accept-Encoding/i);assert.match(headers,/server-timing: layer;dur=/i);assert.ok(data.timing.queries>0);
     assert.ok(data.sessions.every(r=>!('first_prompt' in r)&&!('prompt_ts' in r)&&!('tool_names' in r)&&Array.from(r.first_prompt_short).length<=600));
    }
   }finally{await stop(server.child);}
   server=await start();
   try{
    const context=await browser.newContext({viewport:{width:1440,height:1000}});
    const page=await context.newPage();
    await page.addInitScript(()=>{
     new MutationObserver(()=>{
      if(!window.rowsAdded&&document.querySelector('#v-traces tbody tr')){
       window.rowsAdded=performance.now();requestAnimationFrame(()=>requestAnimationFrame(()=>window.rowsPaint=performance.now()));
      }
     }).observe(document,{childList:true,subtree:true});
    });
    for(const temperature of ['cold','warm']){
     if(temperature==='cold')await page.goto(server.base+'/?window=30d#traces',{waitUntil:'domcontentloaded'});
     else await page.reload({waitUntil:'domcontentloaded'});
     const loading=await page.locator('#archive-status').innerText()==='Loading…';
     await page.waitForFunction(()=>window.rowsPaint&&!document.body.classList.contains('loading'));
     const sample=await page.evaluate(()=>({chrome:performance.getEntriesByType('paint')[0]?.startTime,rows:window.rowsPaint,rowsAdded:window.rowsAdded,requests:performance.getEntriesByType('resource').filter(r=>r.name.includes('/api/')).map(r=>({name:r.name,start:r.startTime,end:r.responseEnd}))}));
     Object.assign(sample,{kind:'browser',round,temperature,base:server.base,initialLoading:loading});report.samples.push(sample);
     check(loading&&sample.chrome<500&&sample.rows<1500,`Browser ${round}/${temperature}: chrome ${sample.chrome}ms, rows ${sample.rows}ms, loading ${loading}`);
     await page.screenshot({path:dir+`/01-${round}-${temperature}-loaded-list.png`});
    }
    await context.close();
   }finally{await stop(server.child);}
  }
 }finally{await browser.close();fs.writeFileSync(dir+'/report.json',JSON.stringify(report,null,2));}
 console.log(JSON.stringify(report,null,2));
 assert.deepEqual(report.failures,[],'latency acceptance failed');
})().catch(e=>{console.error(e);process.exitCode=1;});
