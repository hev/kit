// Run against TestDashboardFixtureServer using HEV_DASHBOARD_URL.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
(async () => {
 const browser=await chromium.launch({headless:true});
 try {
  const page=await browser.newPage();
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const base=process.env.HEV_DASHBOARD_URL||'http://127.0.0.1:18787';
  await page.goto(base+'/?window=all#traces');
  await page.waitForSelector('[data-trace-count="6"]');
  const mode=page.getByRole('combobox',{name:'Search mode'});
  assert.equal(await mode.inputValue(),'hybrid');
  await mode.selectOption('phrase');
  assert.equal(new URL(page.url()).searchParams.get('mode'),'phrase');
  const query=page.getByRole('textbox',{name:'Search traces',exact:true});
  await query.fill('preflight transcript');
  const response=page.waitForResponse(r=>r.url().includes('/api/search?'));
  await query.press('Enter');
  const wire=await response;
  assert.equal(new URL(wire.url()).searchParams.get('mode'),'phrase');
  assert.equal((await wire.json()).mode,'phrase');
  await page.waitForSelector('[data-trace-count="6"]');
  const chip=page.locator('.live-filter:has(> span:text-is("project"))');
  await chip.click();
  await page.locator('#filter-cards .vs-option[title=\'"alpha"\']').click();
  await page.waitForSelector('[data-trace-count="3"]');
  assert.equal(new URL(page.url()).searchParams.get('mode'),'phrase');
  await page.getByRole('button',{name:'7d',exact:true}).click();
  await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.equal(new URL(page.url()).searchParams.get('mode'),'phrase');
  await page.reload();
  await page.waitForSelector('[data-trace-count="3"]');
  assert.equal(await mode.inputValue(),'phrase');
  assert.equal(await query.inputValue(),'preflight transcript');
  // A copied URL initializes the mode without localStorage or prior history.
  const linked=await browser.newPage();
  await linked.goto(page.url());
  await linked.waitForSelector('[data-trace-count="3"]');
  assert.equal(await linked.getByRole('combobox',{name:'Search mode'}).inputValue(),'phrase');
  await linked.close();
  const popResponse=page.waitForResponse(r=>r.url().includes('/api/search?'));
  await mode.selectOption('hybrid');await popResponse;
  assert.equal(new URL(page.url()).searchParams.get('mode'),'hybrid');
  await page.goBack();await page.waitForSelector('[data-trace-count="3"]');
  assert.equal(await mode.inputValue(),'phrase');
  // Make the incomplete-result contract visible even when the match set is small.
  await page.route('**/api/search?**',async route=>{
   const result=await route.fetch();const body=await result.json();
   body.candidate_truncated={transcript:true,eval:false};
   await route.fulfill({response:result,json:body});
  });
  await query.press('Enter');
  await page.waitForFunction(()=>document.querySelector('#archive-status').textContent.includes('results are incomplete'));
  assert.deepEqual(errors,[]);
  console.log('PASS: phrase API mode; URL persistence through filters, timeframe, reload, shared link and history; visible truncation');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exit(1);});
