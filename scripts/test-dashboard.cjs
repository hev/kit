// Run against the Go wire fixture. No application JS build or runtime dependency.
// NODE_PATH=<temporary playwright node_modules> node scripts/test-dashboard.cjs
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
// Filters are layer-ui chips: open the field's picker, then toggle a value in it.
const pickFilter=async(page,key,value)=>{
 const chip=page.locator(`.live-filter:has(> span:text-is("${key}"))`);
 if(await chip.getAttribute('aria-expanded')!=='true')await chip.click();
 await page.locator(`#filter-editor .vs-option[title='${JSON.stringify(value)}']`).click();
};
(async () => {
 const browser=await chromium.launch({headless:true,timeout:30000});
 try {
  const page=await browser.newPage({viewport:{width:1440,height:1000}});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const base=process.env.HEV_DASHBOARD_URL||'http://127.0.0.1:18787';
  await page.goto(base+'/?window=all#traces');
  await page.waitForSelector('[data-trace-count="6"]');
  let searches=0;page.on('request',r=>{if(r.url().includes('/api/search?'))searches++;});
  const search=page.getByRole('textbox',{name:'Search traces',exact:true});
  await search.fill('preflight');await page.waitForTimeout(250);assert.equal(searches,0,'typing issued a search');
  await Promise.all([page.waitForResponse(r=>r.url().includes('/api/search?')),search.press('Enter')]);await page.waitForSelector('th:text-is("Matching snippet")');assert.equal(searches,1);
  await pickFilter(page,'project','alpha');await page.waitForSelector('[data-trace-count="3"]');
  await pickFilter(page,'project','beta');await page.waitForSelector('[data-trace-count="6"]');
  await pickFilter(page,'model','model-a');await page.waitForSelector('[data-trace-count="3"]');
  await pickFilter(page,'tool','Bash');await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  assert.equal(await page.getByRole('button',{name:'Apply ranges',exact:true}).count(),0);
  assert.deepEqual(new URL(page.url()).searchParams.getAll('project'),['alpha','beta']);
  await page.reload();await page.waitForSelector('th:text-is("Matching snippet")');await page.waitForSelector('[data-trace-count="3"]');
  await page.locator('#v-traces a.hit').first().click();await page.waitForSelector('.turn.hl');
  assert.match(page.url(),/#t-assistant-/);assert.ok(await page.locator('.turn.hl').count());
  console.log('PASS: no request while typing; Enter submits; multi-project/model/tool filters survive reload; snippet opens containing turn');

  await page.goto(base+'/?window=all#stats');await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  const spend=page.locator('.chart').filter({has:page.getByRole('heading',{name:'Spend per day',exact:true})});
  const bar=spend.locator('rect.bar').first();await bar.hover();
  const tooltip=await page.locator('#tip').innerText();const count=Number(tooltip.match(/(\d+) traces/)[1]);
  await bar.click();await page.waitForSelector(`[data-trace-count="${count}"]`);
  const dayURL=new URL(page.url());assert.ok(dayURL.searchParams.get('since'));assert.ok(dayURL.searchParams.get('until'));
  await page.goto(base+'/?window=all#stats');await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  const tokens=page.locator('.chart').filter({has:page.getByRole('heading',{name:'Tokens per day by model',exact:true})});
  const segment=tokens.locator('rect.bar').first();await segment.hover();const segmentTip=await page.locator('#tip').innerText();const segmentCount=Number(segmentTip.match(/(\d+) traces/)[1]);
  await segment.click();await page.waitForSelector(`[data-trace-count="${segmentCount}"]`);assert.ok(new URL(page.url()).searchParams.get('model'));
  await page.goto(base+'/?window=all#stats');await page.waitForFunction(()=>!document.body.classList.contains('loading'));
  const cells=page.locator('.heat .c');await cells.first().waitFor();
  const index=await cells.evaluateAll(nodes=>nodes.findIndex(n=>!n.title.includes('· 0 prompts')));assert.ok(index>=0,'heatmap has no prompts');
  const cell=cells.nth(index);const selected=page.waitForResponse(r=>r.url().includes('/api/sessions?')&&r.url().includes('weekday='));await cell.click();const cellCount=(await (await selected).json()).sessions.length;assert.ok(cellCount>0);await page.waitForSelector(`[data-trace-count="${cellCount}"]`);
  console.log(`PASS: spend day (${count}), model segment (${segmentCount}), heatmap cell (${cellCount}) list counts match charts and server selection`);

  await page.goto(base+'/?window=all&poor=true#traces');await page.waitForSelector('[data-trace-count="1"]');assert.equal(await page.locator('#v-traces .pill').filter({hasText:/^poor$/}).innerText(),'poor');
  assert.equal(await page.locator('#v-traces td').nth(9).locator('span').first().innerText(),'2·4·5·5');
  await page.getByRole('textbox',{name:'Search traces',exact:true}).fill('Distinct evaluator phrase');await page.getByRole('button',{name:'Search',exact:true}).click();await page.waitForSelector('#v-traces a.hit');
  await page.locator('#v-traces a.hit').click();await page.waitForSelector('#v-eval.on');
  assert.match(await page.locator('#v-eval').innerText(),/Distinct evaluator phrase/);assert.match(await page.locator('#v-eval').innerText(),/Add an empty-response regression test/);
  await page.getByRole('button',{name:'Turn 1',exact:true}).click();await page.waitForSelector('.turn.hl');
  console.log('PASS: poor-only trace shows four generic marks; evaluator search opens Eval evidence/findings; evidence link opens turn');

  await page.goto(base+'/?window=all#stats');await page.waitForFunction(()=>!document.body.classList.contains('loading'));await page.waitForSelector('.heat .c');
  assert.equal(await page.locator('.wordmark').innerText(),'hev_ kit');assert.equal(await page.title(),'hev_ kit — traces');
  const screenshotDir=process.env.HEV_SCREENSHOT_DIR||'docs/images';fs.mkdirSync(screenshotDir,{recursive:true});
  await page.screenshot({path:screenshotDir+'/dashboard-wordmark.png',animations:'disabled'});
  assert.deepEqual(errors,[],'browser errors');
  console.log('PASS: hev_ kit wordmark/title; screenshot docs/images/dashboard-wordmark.png; no browser errors');
 } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exit(1);});
