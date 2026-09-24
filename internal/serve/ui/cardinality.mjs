// Host-supplied facts, never inferred from a result page or schema type alone.
export const listPresentations=['auto','choices','compact','rich','search','tags','freeform'];
export const numberPresentations=['auto','range','inputs','choices'];
export function resolveList({profile={},presentation='auto',hasSource=true,remoteSearch=false}={}) {
  if(!listPresentations.includes(presentation))presentation='auto';
  if(presentation==='tags')return {presentation:'tags',reason:'Type a value · Enter adds a filter'};
  if(!hasSource)return {presentation:'freeform',reason:'No option source · enter exact values'};
  if(presentation!=='auto')return {presentation:presentation==='choices'&&!hasSource?'freeform':presentation,reason:'Explicit host presentation'};
  const n=profile.cardinality?.quality==='exact'?profile.cardinality.value:!profile.cardinality&&profile.enumeration==='complete'&&Array.isArray(profile.values)?new Set(profile.values).size:null;
  if(profile.enumeration==='complete'&&Number.isSafeInteger(n)&&n>=0&&n<=8)return {presentation:'choices',reason:`${n} values · complete field profile`};
  if(Number.isSafeInteger(n)&&n>8&&n<=50)return {presentation:'compact',reason:`${n} distinct values · compact picker`};
  if(Number.isSafeInteger(n)&&n>50&&n<=1000)return {presentation:'rich',reason:`${n} distinct values · browsable picker`};
  if(remoteSearch)return {presentation:'search',reason:n>1000?`${n.toLocaleString()} values · search the source`:'Unknown cardinality · search the source'};
  return {presentation:'rich',reason:'Partial or unknown vocabulary · browse supplied pages'};
}
export function resolveNumber(s,{hasDomain=false}={}) {
  const profile=s.fieldProfile||{},requested=s.numberPresentation||'auto';
  const choices=profile.enumeration==='complete'&&Array.isArray(profile.values)&&profile.values.length<=50?profile.values:[];
  const categorical=['category','ordinal','identifier'].includes(profile.meaning);
  let presentation=numberPresentations.includes(requested)?requested:'auto';
  if(presentation==='auto')presentation=categorical&&choices.length&&choices.length<=8?'choices':profile.meaning==='identifier'||!hasDomain?'inputs':'range';
  if(presentation==='choices'&&!choices.length)presentation='inputs';
  return {presentation,choices,reason:presentation==='choices'?'Exact values · not a continuous interval':presentation==='range'?'Measurement · supplied slider extent':'Precise values · statistics optional'};
}
export function toggleNumberValue(s,value) {
  const values=s.rangeMode==='set'?[...(s.numberValues||[])]:[];
  const raw=String(value),i=values.findIndex(v=>String(v)===raw);
  if(i<0)values.push(raw);else values.splice(i,1);
  s.numberValues=values;s.rangeMode=values.length?'set':'any';
}
export function listValueError(s) {
  if(!Array.isArray(s.values)||!s.values.every(v=>typeof v==='string'))return 'Enter exact string values.';
  if(s.listSelection==='single'&&s.values.length>1)return 'Choose one value or switch to multiple selection.';
  const p=s.fieldProfile;
  if(p?.authoritativeDomain&&p.enumeration==='complete'&&Array.isArray(p.values)&&s.values.some(v=>!p.values.includes(v)))return 'A selected value is outside the declared vocabulary. Remove it or repair the domain.';
  return '';
}
