// No application dependency: NODE_PATH=<playwright node_modules> node this-file.
// HEV_DASHBOARD_BINARY starts/stops an isolated read-only server for the real
// stopped-server check. Otherwise HEV_DASHBOARD_URL points at the Go fixture.
const {chromium}=require('playwright');
const assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const {once}=require('node:events');
// Filters are layer-ui chips: open the field's picker, then toggle a value in it.
const pickFilter=async(page,key,value)=>{
 const chip=page.locator(`.live-filter:has(> span:text-is("${key}"))`);
 if(await chip.getAttribute('aria-expanded')!=='true')await chip.click();
 await page.locator(`#filter-editor .vs-option[title='${JSON.stringify(value)}']`).click();
};
(async()=>{
 let server;
 const browser=await chromium.launch({headless:true});
 try {
  let base=process.env.HEV_DASHBOARD_URL||'http://127.0.0.1:18787';
  if(process.env.HEV_DASHBOARD_BINARY){
   server=spawn(process.env.HEV_DASHBOARD_BINARY,['serve','--port','0'],{stdio:['ignore','pipe','inherit']});
   base=await new Promise((resolve,reject)=>{
    let output='';server.stdout.on('data',chunk=>{output+=chunk;const m=output.match(/http:\/\/127\.0\.0\.1:\d+/);if(m)resolve(m[0]);});
    server.once('error',reject);server.once('exit',code=>reject(Error('server exited: '+code)));
   });
  }
  const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.addInitScript(()=>{
   new MutationObserver(()=>{
    if(!window.rowsAdded&&document.querySelector('#v-traces tbody tr')){
     window.rowsAdded=performance.now();requestAnimationFrame(()=>requestAnimationFrame(()=>window.rowsFrame=performance.now()));
    }
   }).observe(document,{childList:true,subtree:true});
  });
  // Hold the fixture response until the initial loading assertions finish.
  let releaseInitial;
  if(!server){const gate=new Promise(resolve=>{releaseInitial=resolve;});await page.route('**/api/sessions?**',async route=>{await gate;await route.continue();},{times:1});}
  await page.goto(base+'/?window=30d#traces',{waitUntil:'domcontentloaded'});
  assert.equal(await page.locator('#archive-status').innerText(),'Loading…');
  assert.equal(await page.locator('.top').evaluate(e=>getComputedStyle(e,'::after').height),'2px');
  releaseInitial?.();
  await page.waitForFunction(()=>window.rowsFrame&&!document.body.classList.contains('loading'));
  const paint=await page.evaluate(()=>({chrome:performance.getEntriesByType('paint')[0]?.startTime,rows:window.rowsFrame}));
  console.log('paint ms',paint);
  assert.match(await page.locator('#archive-status').innerText(),/traces · \$.* · Layer [\d.]+ s, \d+ quer/);
  // Delay one response so the existing table's loading state is observable.
  await page.route('**/api/sessions?**',async route=>{await new Promise(r=>setTimeout(r,200));await route.continue();},{times:1});
  const project=process.env.HEV_DASHBOARD_BINARY?'lyr':'alpha';
  await pickFilter(page,'project',project);
  assert.equal(await page.locator('#archive-status').innerText(),'Loading…');
  assert.equal(await page.locator('#v-traces .grid').evaluate(e=>getComputedStyle(e).opacity),'0.4');
  await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.ok(await page.locator('#v-traces tbody tr').count());
  await page.locator('#tabs [data-v="stats"]').click();
  await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.match(await page.locator('#v-stats h2').innerText(),/Layer [\d.]+ s, \d+ quer/);
  await page.locator('.heat button').first().waitFor({state:'visible'});
  assert.equal(await page.locator('.heat button').count(),168);
  const coverage=await page.request.get(base+'/api/stats?window=30d&project='+project);
  const stats=await coverage.json();if(stats.prompt_coverage.with<stats.prompt_coverage.total)assert.match(await page.locator('#v-stats').innerText(),/traces carry prompt times, run hev index --read-side --force/);
  const selected=page.waitForResponse(r=>r.url().includes('/api/sessions?')&&r.url().includes('weekday='));
  await page.locator('.heat button').first().click();const result=await(await selected).json();
  await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.equal(await page.locator('#v-traces').getAttribute('data-trace-count'),String(result.sessions.length));
  if(server){const exited=once(server,'exit');server.kill('SIGTERM');await exited;server=undefined;}
  else await page.route('**/api/**',route=>route.abort('connectionrefused'));
  await page.locator('#range button').first().click();
  await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.match(await page.locator('#archive-status').innerText(),/Could not load traces: .*fetch/i);
  assert.equal(await page.locator('#v-traces .grid').evaluate(e=>getComputedStyle(e).opacity),'1');
  assert.deepEqual(errors,[]);
  if(process.env.HEV_DASHBOARD_BINARY){assert.ok(paint.chrome<500,'chrome exceeded 500ms');assert.ok(paint.rows<1500,'rows exceeded 1500ms');}
  console.log('PASS: initial loading, Layer timing, facet loading/dimming, server heatmap, coverage, failed fetch in count line');
 }finally{if(server)server.kill('SIGTERM');await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
