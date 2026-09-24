import {resolveList} from './cardinality.mjs';
// Reusable vanilla-DOM specimen; no dependency on the workbench or fixture source.
const escape = value => String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
export const valueLabel = value => value === '' ? 'Empty string' : /^\s+$/.test(value) ? `Whitespace only · ${value.length} characters` : value;
export const searchKey = value => value.normalize('NFKD').replace(/\p{M}/gu,'').toLocaleLowerCase();
export function matchOptions(values, query) {
  const needle=searchKey(query.trim());
  return values.filter(row=>searchKey(valueLabel(row.v)).includes(needle)||searchKey(JSON.stringify(row.v)).includes(needle));
}
let instance=0;
export function createValueSelect(host,{value=[],loadValues,disabled=false,label='Values',scopeLabel=null,showCounts,showBars,showSearch,showScope,showDetails,presentation='auto',profile={},remoteSearch=false,selection='multiple',allowCustom=false,onChange=()=>{}}) {
  const id=`value-select-${++instance}`;
  const hasSource=typeof loadValues==='function'||Array.isArray(profile.values);
  presentation=resolveList({profile,presentation,hasSource,remoteSearch}).presentation;
  const minimal=presentation==='tags',inline=presentation==='choices',freeform=minimal||presentation==='freeform';
  showCounts??=presentation==='rich';showBars??=presentation==='rich';showSearch??=!inline;showScope??=presentation==='rich';showDetails??=presentation==='rich';
  if(presentation==='search'&&remoteSearch)showSearch=true;
  allowCustom=allowCustom||freeform;
  if(!loadValues&&Array.isArray(profile.values))loadValues=async()=>({values:profile.values.map(v=>({v})),total:profile.values.length});
  const focusInput=()=>{const el=inline?host.querySelector('[data-option]'):freeform?host.querySelector('.vs-exact-input'):host.querySelector('.vs-input');el?.focus({preventScroll:true});};
  let loadController=null,entryError='';

  let selected=[...value],query='',open=true,rows=[],total=0,truncated=false,hasMore=false,bounded=false,approximate=false,loading=!disabled,error=false,active=-1,inspected=null,requestId=0,disposed=false;
  let page=0,pageSize=50,searching=false,searchTimer,searchVersion=0,pendingLoad=null,lastLoadMore=false,pointerActive=false;
  const abort=new AbortController();
  host.innerHTML=`<div class="value-select vs-${presentation} ${disabled?'is-disabled':''}">
    <div class="vs-label"><label for="${id}-input">${escape(label)}</label><span>${selection==='single'?'Select one value':'Match any selected value'}</span></div>
    <div class="vs-chips" aria-label="Selected values"></div>
    <div class="vs-input-wrap"><span aria-hidden="true">⌕</span><input id="${id}-input" class="vs-input" role="combobox" aria-autocomplete="list" aria-expanded="true" aria-controls="${id}-options" aria-describedby="${id}-scope ${id}-status" placeholder="Search values…" autocomplete="off" spellcheck="false" ${disabled?'disabled':''}><button type="button" class="vs-toggle" aria-label="Toggle options" aria-controls="${id}-options" ${disabled?'disabled':''}>⌃</button></div>
    <div class="vs-menu"><div class="vs-scope"  id="${id}-scope">${escape(scopeLabel)}</div><div class="vs-options" id="${id}-options" role="listbox" aria-label="${escape(label)}" aria-multiselectable="${selection!=='single'}"></div><div class="vs-status" id="${id}-status" role="status"></div><div class="vs-pagination"></div><div class="vs-detail" hidden></div><div class="vs-menu-footer"><span>↑↓ navigate · Enter select · Esc close</span><button type="button" class="vs-done">Done</button></div></div>
    ${minimal?`<div class="vs-token-entry"><input id="${id}-exact" class="vs-exact-input" placeholder="Type a value and press Enter…" aria-describedby="${id}-entry-error" autocomplete="off" spellcheck="false" enterkeyhint="done" ${disabled?'disabled':''}></div><p id="${id}-entry-error" class="vs-entry-error" role="status"></p>`:allowCustom?`<div class="vs-exact"><label for="${id}-exact">Exact value</label><div><input id="${id}-exact" class="vs-exact-input" autocomplete="off" spellcheck="false" ${disabled?'disabled':''}><button type="button" class="vs-add-exact" ${disabled?'disabled':''}>Add value</button></div><button type="button" class="vs-add-empty" ${disabled?'disabled':''}>Add empty string</button><small>Exact match · no value is created in the data source.</small><p class="vs-entry-error" role="status"></p></div>`:''}
    <div class="vs-summary"></div>
  </div>`;
  const $=s=>host.querySelector(s),input=$('.vs-input');
  if(!showSearch){input.readOnly=true;input.placeholder='Select values…';input.setAttribute('aria-autocomplete','none');}
  $('.vs-scope').hidden=!showScope;
  $('.vs-input-wrap').hidden=inline||freeform;
  $('.vs-menu-footer').hidden=inline||freeform;
  if(inline||freeform){$('.vs-label label').removeAttribute('for');$('.vs-options').setAttribute('aria-label',label);}
  if(minimal){$('.vs-label label').htmlFor=id+'-exact';$('.vs-label>span').hidden=true;$('.vs-token-entry').prepend($('.vs-chips'));$('.vs-summary').hidden=true;}
  input.setAttribute('aria-describedby',`${showScope?id+'-scope ':''}${id}-status`);
  const filtered=()=>remoteSearch?rows:matchOptions(rows,query);
  const visibleRows=()=>filtered().slice(page*pageSize,(page+1)*pageSize);
  function chips() {
    $('.vs-chips').innerHTML=selected.map((v,i)=>`<span class="vs-chip">${minimal?`<span class="vs-chip-label" title="${escape(JSON.stringify(v))}"><bdi>${escape(valueLabel(v))}</bdi></span>`:`<button type="button" data-inspect="${i}" class="vs-chip-label" title="${escape(JSON.stringify(v))}" aria-label="${showDetails?'Inspect selected value':'Selected value'} ${escape(valueLabel(v))}" ${disabled?'disabled':''}><bdi>${escape(valueLabel(v))}</bdi></button>`}<button type="button" data-remove="${i}" class="vs-chip-remove" aria-label="Remove ${escape(valueLabel(v))}" ${disabled?'disabled':''}>×</button></span>`).join('');
    $('.vs-chips').hidden=!selected.length;
    $('.vs-summary').innerHTML=`<span>${selected.length?`${selected.length} selected${selection==='single'?'':' · combined with OR'}`:'Any value · no filter applied'}</span>${selected.length?`<button type="button" class="vs-clear" ${disabled?'disabled':''}>Clear all</button>`:''}`;
  }
  function detail() {
    const target=inspected ?? (active>=0?visibleRows()[active]?.v:null);
    $('.vs-detail').hidden=!showDetails||target===null||target===undefined;
    if(target!==null&&target!==undefined) $('.vs-detail').innerHTML=`<div><span>EXACT VALUE · ${target.length} CHARACTERS</span><button type="button" class="vs-copy">Copy value</button></div><code dir="auto">${escape(JSON.stringify(target))}</code>`;
  }
  function menu() {
    const matches=filtered();
    const visible=visibleRows();
    // Use all loaded values, so typeahead and selection do not rescale the bars.
    const hasCount=row=>typeof row.n==='number'&&Number.isFinite(row.n)&&row.n>=0;
    const counted=rows.some(hasCount);
    $('.vs-scope').textContent=scopeLabel||(counted?'Matching documents · before this filter':'Available values · counts not supplied');
    $('.vs-scope').title=counted&&showBars?'Bars compare counts with the largest loaded value, not the sum of counts.':'';
    const maxCount=rows.reduce((max,row)=>Number.isFinite(row.n)?Math.max(max,row.n):max,0);
    if(active>=visible.length) active=visible.length-1;
    $('.vs-menu').hidden=freeform||!open;
    input.setAttribute('aria-expanded',String(open));
    $('.vs-toggle').setAttribute('aria-expanded',String(open));
    $('.vs-toggle').textContent=open?'⌃':'⌄';
    if(open&&active>=0&&(!loading||rows.length>0)&&!error)input.setAttribute('aria-activedescendant',`${id}-option-${active}`);else input.removeAttribute('aria-activedescendant');
    $('.vs-options').setAttribute('aria-busy',String(loading));
    if(disabled) $('.vs-options').innerHTML='<div class="vs-message">Bind a compatible field to load values.</div>';
    else if(loading&&!rows.length) $('.vs-options').innerHTML='<div class="vs-message"><span class="vs-spinner"></span>Loading values…<small>Your selected values are preserved.</small></div>';
    else if(error) $('.vs-options').innerHTML='<div class="vs-message">Couldn’t load values.<small>Your selection is still applied.</small><button type="button" class="vs-retry">Retry</button></div>';
    else if(!visible.length) $('.vs-options').innerHTML=`<div class="vs-message">${searching?'Searching remaining values…':query?'No matching values'+(!remoteSearch&&(truncated||hasMore)?' in the loaded list':''):presentation==='search'?'Type to search values':truncated||hasMore?'No values in this page':'No values in this scope'}${searching?'':'.'}<small>${query?'Try a shorter term. Selected chips stay applied.':'Change the query or another filter to widen the scope.'}</small>${query?'<button type="button" class="vs-reset-search">Clear option search</button>':''}</div>`;
    else $('.vs-options').innerHTML=visible.map((row,i)=>`<div role="option" id="${id}-option-${i}" data-option="${i}" ${inline?'tabindex="0"':''} aria-selected="${selected.includes(row.v)}" class="vs-option ${i===active?'is-active':''}" title="${escape(JSON.stringify(row.v))}">${showBars&&hasCount(row)?`<span class="vs-bar-track" aria-hidden="true"><span class="vs-bar" style="width:${maxCount>0?row.n/maxCount*100:0}%"></span></span>`:''}<span class="vs-check" aria-hidden="true">${selected.includes(row.v)?'✓':''}</span><span class="vs-option-label"><bdi>${escape(valueLabel(row.v))}</bdi>${row.v!==row.v.trim()&&row.v.trim()?'<small>Includes surrounding whitespace</small>':''}</span>${showCounts&&counted?`<span class="vs-count" aria-label="${hasCount(row)?`${bounded?'at least ':approximate?'approximately ':''}${row.n} documents`:'Document count unavailable'}">${hasCount(row)?`${bounded?'≥ ':approximate?'≈ ':''}${row.n.toLocaleString()}`:'—'}</span>`:''}</div>`).join('');
    $('.vs-status').innerHTML=disabled?'No option request':error?'Options unavailable · selected values retained':searching?`Searching values · ${rows.length.toLocaleString()} / ${total.toLocaleString()} loaded`:loading?'Loading next values…':`${query?`${remoteSearch?total:matches.length} matching · `:''}${rows.length.toLocaleString()} / ${total.toLocaleString()} values loaded${truncated?' · source listing truncated':''}${bounded?' · counts are lower bounds':''}${approximate?' · approximate membership':''}`;
    const count=remoteSearch?total:query?matches.length:truncated?rows.length:total;
    const canNext=(page+1)*pageSize<matches.length||((!query||remoteSearch)&&hasMore);
    $('.vs-pagination').hidden=disabled||(!rows.length&&!hasMore)||(total<=25&&!query);
    $('.vs-pagination').innerHTML=`<div><span>${visible.length?`${(page*pageSize+1).toLocaleString()}–${(page*pageSize+visible.length).toLocaleString()}`:'0'} of ${count.toLocaleString()}${query&&hasMore&&!remoteSearch?' +':''}</span><label>Per page <select class="vs-page-size" aria-label="Values per page">${[25,50,100].map(n=>`<option ${n===pageSize?'selected':''}>${n}</option>`).join('')}</select></label></div><div><button type="button" class="vs-previous" ${page===0||loading?'disabled':''} aria-label="Previous values page">←</button><span>Page ${page+1} / ${Math.max(1,Math.ceil(count/pageSize))}${query&&hasMore&&!remoteSearch?' +':''}</span><button type="button" class="vs-next" ${!canNext||loading?'disabled':''} aria-label="Next values page">→</button></div>`;
    if(!remoteSearch&&!searching&&!loading&&!error&&hasMore&&query) $('.vs-status').innerHTML+='<button type="button" class="vs-search-all">Search remaining values</button>';
    if(inline&&!loading&&!error&&total<=8&&!truncated){$('.vs-status').hidden=true;$('.vs-pagination').hidden=true;}else $('.vs-status').hidden=false;
    if(!remoteSearch&&hasMore)input.placeholder='Search loaded values…';
    if($('.vs-entry-error'))$('.vs-entry-error').textContent=entryError||(minimal&&selection==='single'&&selected.length>1?'Choose one value · existing selections preserved for repair':'');
    if(selection==='single'&&selected.length>1)$('.vs-summary span').textContent='Choose one value · existing selections preserved for repair';
    detail();
  }
  function apply(v) {
    selected=selected.includes(v)?selected.filter(item=>item!==v):selection==='single'?[v]:[...selected,v];
    inspected=v;chips();menu();onChange([...selected]);
  }
  async function load(more=false) {
    if(disabled||disposed||freeform||!loadValues)return false;
    if(more&&pendingLoad)return pendingLoad;
    loadController?.abort();loadController=new AbortController();
    const revision=++requestId;loading=true;error=false;if(!more)active=-1;lastLoadMore=more;menu();
    const task=(async()=>{
      try {
        const result=await loadValues({query:remoteSearch?query:undefined,offset:more?rows.length:0,limit:100,signal:loadController.signal});
        if(disposed||revision!==requestId)return false;
        if(!Array.isArray(result.values)||!result.values.every(row=>row&&typeof row.v==='string'))throw new Error('Expected exact string values');
        const previous=more?rows:[];
        rows=[...new Map([...previous,...result.values].map(row=>[row.v,row])).values()];
        total=Number.isSafeInteger(result.total)?result.total:rows.length;truncated=!!result.truncated;
        hasMore=!truncated&&result.values.length>0&&rows.length<total;
        bounded=!!result.bounded;approximate=!!result.approximate;loading=false;menu();
        return true;
      } catch(e) {if(disposed||revision!==requestId)return false;loading=false;error=true;menu();return false;}
      finally {if(revision===requestId)pendingLoad=null;}
    })();
    pendingLoad=task;return task;
  }
  async function searchAll(version) {
    if(disposed||version!==searchVersion||!query)return;
    searching=true;menu();
    while(!disposed&&version===searchVersion&&hasMore) {if(!await load(true))break;}
    if(!disposed&&version===searchVersion){searching=false;menu();}
  }
  function searchChanged() {
    query=input.value;open=true;page=0;active=-1;inspected=null;
    clearTimeout(searchTimer);const version=++searchVersion;searching=false;
    if(remoteSearch){loadController?.abort();requestId++;pendingLoad=null;rows=[];total=0;hasMore=false;loading=!!query;menu();if(query)searchTimer=setTimeout(()=>load(),250);}else menu();
  }
  async function changePage(direction) {
    const next=page+direction;if(next<0)return;focusInput();
    if(direction>0&&(!query||remoteSearch)&&next*pageSize>=rows.length&&hasMore) {if(!await load(true))return;}
    if(disposed)return;
    if(next*pageSize>=filtered().length)return;
    page=next;active=-1;inspected=null;menu();$('.vs-options').scrollTop=0;focusInput();
  }
  function close(){if(inline||freeform)return;open=false;menu();}
  function click(event) {
    if(disabled)return;
    if(event.target.closest('.vs-add-exact')){addExact();return;}
    if(event.target.closest('.vs-add-empty')){addExact('');return;}
    const option=event.target.closest('[data-option]');
    if(option){active=Number(option.dataset.option);apply(visibleRows()[active].v);(inline?host.querySelector(`[data-option="${active}"]`):input)?.focus({preventScroll:true});return;}
    const button=event.target.closest('button');if(!button){if(minimal&&event.target.closest('.vs-token-entry'))$('.vs-exact-input').focus();return;}
    if(button.classList.contains('vs-toggle')){open=!open;menu();focusInput();}
    if(button.classList.contains('vs-done')){close();focusInput();}
    if(button.dataset.remove!==undefined){const index=Number(button.dataset.remove);selected.splice(index,1);chips();menu();onChange([...selected]);const next=host.querySelectorAll('[data-remove]')[Math.min(index,selected.length-1)];if(next)next.focus({preventScroll:true});else focusInput();}
    if(showDetails&&button.dataset.inspect!==undefined){inspected=selected[Number(button.dataset.inspect)];open=true;menu();}
    if(button.classList.contains('vs-clear')){selected=[];chips();menu();onChange([]);focusInput();}
    if(button.classList.contains('vs-reset-search')){input.value='';searchChanged();input.focus();}
    if(button.classList.contains('vs-retry')){focusInput();load(lastLoadMore);}
    if(button.classList.contains('vs-search-all')){focusInput();searchAll(++searchVersion);}
    if(button.classList.contains('vs-previous'))changePage(-1);
    if(button.classList.contains('vs-next'))changePage(1);
    if(button.classList.contains('vs-copy')){const target=inspected??visibleRows()[active]?.v;navigator.clipboard.writeText(target).then(()=>{button.textContent='Copied';}).catch(()=>{button.textContent='Select text to copy';});}
  }
  function addExact(value=$('.vs-exact-input')?.value){
    if(disabled||value===undefined)return;
    if(value===''&&arguments.length===0){if(minimal){entryError='';menu();return;}entryError='Use Add empty string to select an empty value.';menu();return;}
    if(profile.authoritativeDomain&&profile.enumeration==='complete'&&Array.isArray(profile.values)&&!profile.values.includes(value)){entryError='This value is outside the declared vocabulary.';menu();return;}
    entryError='';if(!selected.includes(value))selected=selection==='single'?[value]:[...selected,value];
    $('.vs-exact-input').value='';chips();menu();onChange([...selected]);$('.vs-exact-input').focus();
  }
  function keydown(event) {
    if(event.isComposing)return;
    if(minimal&&event.target.classList.contains('vs-exact-input')&&event.key==='Backspace'&&!event.target.value&&selected.length){event.preventDefault();host.querySelectorAll('[data-remove]')[selected.length-1]?.focus();return;}
    if(event.target.classList.contains('vs-exact-input')&&event.key==='Enter'){event.preventDefault();event.stopPropagation();addExact();return;}
    const option=event.target.closest('[data-option]');
    if(inline&&option){
      const index=Number(option.dataset.option);
      if(['Enter',' '].includes(event.key)){event.preventDefault();active=index;apply(visibleRows()[index].v);host.querySelector(`[data-option="${index}"]`)?.focus();}
      if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)){event.preventDefault();const count=visibleRows().length,next=event.key==='Home'?0:event.key==='End'?count-1:Math.max(0,Math.min(count-1,index+(event.key==='ArrowDown'?1:-1)));host.querySelector(`[data-option="${next}"]`)?.focus();}
      return;
    }
    if(event.target!==input)return;
    if(event.isComposing)return;
    if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)&&(event.key.startsWith('Arrow')||open)){
      event.preventDefault();open=true;inspected=null;const length=visibleRows().length;
      if(length){active=event.key==='Home'?0:event.key==='End'?length-1:event.key==='ArrowDown'?Math.min(active+1,length-1):active<0?length-1:Math.max(0,active-1);}
      menu();host.querySelector('.is-active')?.scrollIntoView({block:'nearest'});
    }
    if(event.key==='Enter'){event.preventDefault();if(!open){open=true;menu();}else if((!loading||rows.length>0)&&!error&&active>=0)apply(visibleRows()[active].v);}
    if(event.key==='Escape'){event.preventDefault();close();}
  }
  input.addEventListener('input',searchChanged,{signal:abort.signal});
  host.addEventListener('change',event=>{if(event.target.classList.contains('vs-page-size')){pageSize=Number(event.target.value);page=0;active=-1;inspected=null;menu();$('.vs-page-size').focus({preventScroll:true});}},{signal:abort.signal});
  input.addEventListener('click',()=>{if(!open){open=true;menu();}},{signal:abort.signal});
  host.addEventListener('click',click,{signal:abort.signal});
  host.addEventListener('keydown',keydown,{signal:abort.signal});
  host.addEventListener('pointerdown',event=>{if(event.target.closest('[data-option]'))event.preventDefault();},{signal:abort.signal});
  document.addEventListener('pointerdown',()=>{pointerActive=true;},{signal:abort.signal});
  document.addEventListener('pointerup',()=>{setTimeout(()=>pointerActive=false,0);},{signal:abort.signal});
  document.addEventListener('click',event=>{if(!event.composedPath().includes(host))close();},{signal:abort.signal});
  host.addEventListener('focusout',event=>{if(pointerActive||host.contains(event.relatedTarget))return;setTimeout(()=>{if(!disposed&&!host.contains(document.activeElement))close();},0);},{signal:abort.signal});
  if(presentation==='search'&&remoteSearch||freeform)loading=false;
  chips();menu();if(presentation!=='search'||!remoteSearch)load();
  return {destroy(){disposed=true;abort.abort();loadController?.abort();requestId++;searchVersion++;clearTimeout(searchTimer);}};
}
