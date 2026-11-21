import schema from '../../public/schemas/kit-dot.schema.json' with { type: 'json' };
export { schema };
export const defaults = () => Object.fromEntries(Object.entries(schema.properties).map(([key, rule]) => [key, structuredClone(rule.default)]));
// Small validator for the JSON Schema keywords used by this version's schema.
// The public schema is the single source of constraints, also usable by other validators.
export function validate(value, rule = schema, path = 'config') {
  if (rule.anyOf) return rule.anyOf.some(r => validate(value, r, path).length === 0) ? [] : [`${path}: invalid value`];
  const errors = [];
  const fail = message => errors.push(`${path}: ${message}`);
  if ('const' in rule && value !== rule.const) fail(`must be ${rule.const}`);
  if (rule.enum && !rule.enum.includes(value)) fail(`choose ${rule.enum.join(', ')}`);
  const type = value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value;
  if (rule.type && type !== rule.type) return [`${path}: must be ${rule.type}`];
  if (type === 'string') {
    if (value.length < (rule.minLength ?? 0) || value.length > (rule.maxLength ?? Infinity)) fail(`length must be ${rule.minLength ?? 0}–${rule.maxLength ?? 'unlimited'}`);
    if (rule.pattern && !new RegExp(rule.pattern).test(value)) fail('invalid format');
  }
  if (type === 'array') {
    if (value.length < (rule.minItems ?? 0) || value.length > (rule.maxItems ?? Infinity)) fail('wrong number of entries');
    if (rule.uniqueItems && new Set(value).size !== value.length) fail('remove duplicate entries');
    value.forEach((v, i) => errors.push(...validate(v, rule.items, `${path}[${i}]`)));
  }
  if (type === 'object') {
    for (const key of rule.required ?? []) if (!(key in value)) fail(`missing ${key}`);
    for (const key of Object.keys(value)) {
      if (rule.properties?.[key]) errors.push(...validate(value[key], rule.properties[key], `${path}.${key}`));
      else if (rule.additionalProperties === false) fail(`unknown field ${key}`);
    }
  }
  return errors;
}
export function validateDimensions(width, height) {
  if (width !== height) throw new Error('Choose a square PNG (equal width and height).');
  if (width < 512) throw new Error('Choose a PNG at least 512 × 512 pixels.');
}
export async function sha256(bytes) {
  return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), b => b.toString(16).padStart(2, '0')).join('');
}
export const iconSizes = { home: 192, large: 512, maskable: 512, apple: 180 };
// Compare visible pixels outside the central 80% circle to a flat corner background.
export function unsafeArtwork({ data, width, height }) {
  const bg = [0, 1, 2].map(c => (data[c] + data[(width - 1) * 4 + c] + data[(height - 1) * width * 4 + c] + data[(width * height - 1) * 4 + c]) / 4);
  const transparent = [3, (width - 1) * 4 + 3, (height - 1) * width * 4 + 3, (width * height - 1) * 4 + 3].some(i => data[i] < 240);
  let outside = 0, content = 0;
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
    if (Math.hypot(x + .5 - width / 2, y + .5 - height / 2) <= width * .4) continue;
    outside++;
    const i = (y * width + x) * 4;
    if (data[i + 3] > 24 && (transparent || bg.some((c, k) => Math.abs(data[i + k] - c) > 32))) content++;
  }
  return content > outside * .01;
}
