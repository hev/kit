// Mini-only visual acceptance. Do not run a browser on Oven.
// NODE_PATH=<playwright modules> HEV_DASHBOARD_URL=<isolated candidate/fixture> node scripts/test-stats-browser.cjs
const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000},timezoneId:'America/Denver'});
  const errors=[],requests=[];page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>requests.push(r.url()));
  const base=process.env.HEV_DASHBOARD_URL||'http://127.0.0.1:18787';
  const response=page.waitForResponse(r=>r.url().includes('/api/stats?'));
  await page.goto(base+'/?window=all#stats');const stats=await (await response).json();
  await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.equal(requests.filter(u=>u.includes('/api/sessions?')).length,0,'cold Stats downloaded sessions');
  assert.equal(requests.filter(u=>u.includes('/api/values?')).length,0,'cold Stats launched picker scans');
  assert.equal(stats.sessions,undefined);assert.equal(stats.corpus,undefined);assert.equal(stats.timezone,'America/Denver');
  assert.equal(await page.locator('#v-stats .tile').count(),6);assert.equal(await page.locator('#v-stats .heat .c').count(),168);
  assert.equal(await page.locator('#v-stats tbody tr').count(),stats.projects.length);
  assert.match(await page.locator('#v-stats').innerText(),/Rates, USD per million tokens/);
  if(process.env.HEV_STATS_SCREENSHOT)await page.screenshot({path:process.env.HEV_STATS_SCREENSHOT,fullPage:true});
  if(stats.projects.length){
   const list=page.waitForResponse(r=>r.url().includes('/api/sessions?'));
   await page.locator('#v-stats tbody tr').first().click();const data=await (await list).json();
   await page.waitForFunction(()=>!document.body.classList.contains('loading'));
   assert.ok(new URL(page.url()).searchParams.has('project'));
   assert.equal(data.stats.traces,data.sessions.length);
   const before=requests.filter(u=>u.includes('/api/stats?')).length;
   await page.locator('#tabs button[data-v="stats"]').click();
   await page.waitForFunction(()=>document.querySelector('#v-stats').classList.contains('on'));
   assert.equal(requests.filter(u=>u.includes('/api/stats?')).length,before,'paired list triggered second Stats scan');
  }
  assert.deepEqual(errors,[]);console.log('PASS: Mini Stats-first, compact independent page, all cards/projects/heatmap/rates, project drilldown and paired list snapshot');
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exit(1);});
