import { test } from 'node:test';
import assert from 'node:assert/strict';
import { defaults, validate, validateDimensions, sha256, unsafeArtwork, iconSizes } from '../src/lib/kit-dot.mjs';
import { createHash } from 'node:crypto';

test('default config validates with a stock icon and dry-run', () => {
  const value = defaults(); assert.deepEqual(validate(value), []); assert.equal(value.icon, null); assert.equal(value.acts.live, false);
  value.wordmark[0] = 'changed'; assert.equal(defaults().wordmark[0], 'kit');
});
test('schema rejects invalid fields and unknown properties', () => {
  for (const [key, value] of Object.entries({version:2,name:' ',wordmark:['one'],operator:'',color:'red',voice:'unknown',logins:[],hosts:['bad host'],icon:{file:'other.png',sha256:'bad'},acts:{live:'yes'},extra:true})) {
    assert.ok(validate({...defaults(), [key]:value}).length, key);
  }
  assert.ok(validate({...defaults(), logins:['you@example.com','you@example.com']}).length);
  assert.ok(validate({...defaults(), logins:['not-an-email']}).length);
  const missing = defaults(); delete missing.name; assert.ok(validate(missing).length);
});
test('square 1024 PNG dimensions accepted; small and non-square rejected', () => {
  assert.doesNotThrow(() => validateDimensions(1024,1024));
  assert.throws(() => validateDimensions(300,300), /at least 512/);
  assert.throws(() => validateDimensions(1024,512), /square/);
  assert.deepEqual(iconSizes, {home:192,large:512,maskable:512,apple:180});
});
test('SHA-256 matches independent digest and stays stable', async () => {
  const bytes = new TextEncoder().encode('kit icon'); const hash = await sha256(bytes);
  assert.equal(hash, createHash('sha256').update(bytes).digest('hex')); assert.equal(await sha256(bytes), hash);
  assert.deepEqual(validate({...defaults(),icon:{file:'icon.png',sha256:hash}}), []);
});
test('safe-zone warns for visible corner content, not centred art or flat background', () => {
  const width = 100, height = 100; const data = new Uint8ClampedArray(width*height*4);
  for(let y=40;y<60;y++) for(let x=40;x<60;x++) data.set([255,0,0,255],(y*width+x)*4);
  assert.equal(unsafeArtwork({data,width,height}),false);
  for(let y=0;y<25;y++) for(let x=0;x<25;x++) data.set([255,0,0,255],(y*width+x)*4);
  assert.equal(unsafeArtwork({data,width,height}),true);
  for(let i=0;i<data.length;i+=4) data.set([255,255,255,255],i);
  assert.equal(unsafeArtwork({data,width,height}),false);
  for(let y=5;y<25;y++) for(let x=5;x<25;x++) data.set([0,0,0,255],(y*width+x)*4);
  assert.equal(unsafeArtwork({data,width,height}),true);
});
