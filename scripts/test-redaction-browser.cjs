// Optional browser stage of TestLiveRedactionAcceptance. Install Playwright
// outside the checkout; the test supplies its isolated dashboard and specimen file.
const {chromium}=require('playwright');
const fs=require('node:fs');
const assert=require('node:assert/strict');
const [base,specimens,lane]=process.argv.slice(2);
const samples=JSON.parse(fs.readFileSync(specimens,'utf8'));
const dir=process.env.HEV_REDACTION_EVIDENCE_DIR;
(async()=>{
 const browser=await chromium.launch({headless:true});
 const report={lane,browser:browser.version(),sessions:[],samples:samples.length};
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}});
  const failures=[];page.on('pageerror',e=>failures.push(e.message));
  for(const session of ['redaction-claude','redaction-codex']){
   await page.goto(`${base}/?window=all&session=${session}#transcript`,{waitUntil:'networkidle'});
   await page.waitForFunction(()=>!document.body.classList.contains('loading')&&document.body.innerText.includes('[REDACTED:'));
   await page.evaluate(()=>document.querySelectorAll('details').forEach(d=>d.open=true));
   const text=await page.locator('body').innerText();
   for(const sample of samples)assert.ok(!text.includes(sample.Secret),`rendered original for ${sample.Rule}`);
   assert.ok(text.includes('[REDACTED:'),'no rendered markers');
   report.sessions.push({session,rendered_markers:(text.match(/\[REDACTED:/g)||[]).length});
   if(dir){fs.mkdirSync(dir,{recursive:true});await page.screenshot({path:`${dir}/${lane}-${session}.png`});}
  }
  assert.deepEqual(failures,[],'browser errors');
  if(dir)fs.writeFileSync(`${dir}/${lane}-browser.json`,JSON.stringify(report,null,2));
  console.log(JSON.stringify(report));
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
