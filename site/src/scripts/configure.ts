import { defaults, validateDimensions, sha256, unsafeArtwork, iconSizes } from '../lib/kit-dot.mjs';
const $ = (id: string) => document.getElementById(id)!;
const form = $('config-form') as HTMLFormElement;
const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement;
let generation = 0;
let urls: string[] = [];
function update() {
  const config = defaults();
  for (const name of ['name', 'color', 'voice']) config[name] = field(name).value.trim();
  config.wordmark = [field('word0').value.trim(), field('word1').value.trim()];
  $('dot-name').textContent = config.name;
  $('home-name').textContent = config.name;
  $('wordmark').replaceChildren(document.createTextNode(config.wordmark[0] + ' '));
  const span = document.createElement('span'); span.textContent = config.wordmark[1]; $('wordmark').append(span);
  if (/^#[a-f\d]{6}$/i.test(config.color)) { $('phone').style.setProperty('--accent', config.color); ( $('picker') as HTMLInputElement).value = config.color; }
  $('voice-label').textContent = `${config.voice} · sample transcript`;
}
form.addEventListener('submit', e => e.preventDefault());
form.addEventListener('input', update);
$('picker').addEventListener('input', () => {field('color').value = ($('picker') as HTMLInputElement).value; update();});
function clearIcon() {
  urls.forEach(url => URL.revokeObjectURL(url)); urls = [];
  document.querySelectorAll<HTMLImageElement>('.icon-preview, #favicon-preview').forEach(img => img.src = '/img/foreman-head.png');
  $('cuts').replaceChildren();
}
$('reset-icon').addEventListener('click', () => {generation++; clearIcon(); ($('icon-input') as HTMLInputElement).value = ''; $('icon-status').textContent = 'Using the default hard-hat hev head. Teams can swap it.'; update();});
async function upload(file: File) {
  const current = ++generation; clearIcon(); $('icon-status').textContent = 'Processing icon…'; update();
  let bitmap: ImageBitmap | undefined;
  try {
    const bytes = await file.arrayBuffer();
    const signature = new Uint8Array(bytes.slice(0, 8));
    if (signature.join(',') !== '137,80,78,71,13,10,26,10') throw new Error('Choose a PNG image.');
    bitmap = await createImageBitmap(file);
    validateDimensions(bitmap.width, bitmap.height);
    const hash = await sha256(bytes);
    if (current !== generation) return;
    const sample = document.createElement('canvas'); sample.width = sample.height = 128;
    const ctx = sample.getContext('2d')!; ctx.drawImage(bitmap, 0, 0, 128, 128);
    const warning = unsafeArtwork(ctx.getImageData(0, 0, 128, 128));
    const cuts: {label: string; size: number; blob: Blob}[] = [];
    for (const [label, size] of Object.entries({...iconSizes, favicon: 32})) {
      const canvas = document.createElement('canvas'); canvas.width = canvas.height = size as number;
      canvas.getContext('2d')!.drawImage(bitmap, 0, 0, size as number, size as number);
      const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob(b => b ? resolve(b) : reject(new Error('Could not cut icon.')), 'image/png'));
      cuts.push({label, size: size as number, blob});
    }
    if (current !== generation) return;
    for (const {label, size, blob} of cuts) {
      const url = URL.createObjectURL(blob); urls.push(url);
      if (label === 'home') document.querySelectorAll<HTMLImageElement>('.orb, .home-icon').forEach(img => img.src = url);
      const id = {maskable: 'mask-preview', apple: 'apple-preview', favicon: 'favicon-preview'}[label];
      if (id) ($(id) as HTMLImageElement).src = url;
      if (label !== 'favicon') { const li = document.createElement('li'); li.textContent = `${label} · ${size} × ${size}`; li.title = `icon-${label}-${hash}.png`; $('cuts').append(li); }
    }
    $('icon-status').textContent = warning ? 'Warning: artwork outside the centre 80% safe circle may be clipped by Android’s round mask.' : 'Icon ready. Artwork fits the sampled safe zone.';
  } catch (error) {
    if (current === generation) { clearIcon(); $('icon-status').textContent = error instanceof Error ? error.message : 'Could not read this PNG.'; }
  } finally { bitmap?.close(); if (current === generation) {update();} }
}
$('icon-input').addEventListener('change', e => {const file = (e.target as HTMLInputElement).files?.[0]; if (file) void upload(file);});
$('drop').addEventListener('dragover', e => e.preventDefault());
$('drop').addEventListener('drop', e => {e.preventDefault(); const files = (e as DragEvent).dataTransfer?.files; if (files?.length === 1) void upload(files[0]); else $('icon-status').textContent = 'Choose one PNG at a time.';});
update();
