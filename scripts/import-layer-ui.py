#!/usr/bin/env python3
"""Verify a hev/layer-ui runtime build and stage the parts the dashboard uses.

The dashboard borrows layer-ui's value picker and palette rather than keeping its
own copy. Build the runtime in a layer-ui checkout (`npm run build:runtime`),
then run this with that `dist-runtime/` directory. Files are checked against the
bundle manifest before they are copied into internal/serve/ui/, which the server
embeds and serves at /ui/. Never edit the staged files: change layer-ui and
import again, so kit and puff stay on the same components.
"""
import argparse
import hashlib
import json
from pathlib import Path

# What template.html loads. Add a file here only when the page starts using it.
FILES = ['list-select.mjs', 'cardinality.mjs', 'list-select.css', 'theme.css']

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument('bundle', type=Path, help='layer-ui dist-runtime directory')
args = parser.parse_args()

manifest = json.loads((args.bundle / 'manifest.json').read_text())
if manifest.get('protocol') != 1 or not manifest.get('version'):
    parser.error('unsupported bundle manifest')
listed = manifest.get('files', {})
contents = {}
for name in FILES:
    if name not in listed:
        parser.error('bundle does not list ' + name)
    data = (args.bundle / name).read_bytes()
    if hashlib.sha256(data).hexdigest() != listed[name]:
        parser.error('checksum mismatch: ' + name)
    contents[name] = data

destination = Path(__file__).resolve().parent.parent / 'internal/serve/ui'
destination.mkdir(parents=True, exist_ok=True)
for name, data in contents.items():
    (destination / name).write_bytes(data)
staged = {
    'protocol': manifest['protocol'],
    'version': manifest['version'],
    # hev/layer-ui is private; kit is private too. Revisit before kit ships publicly.
    'redistributable': manifest.get('redistributable', False),
    'files': {name: listed[name] for name in FILES},
}
(destination / 'manifest.json').write_text(json.dumps(staged, indent=2) + '\n')
print('Staged layer-ui ' + manifest['version'] + ' (' + ', '.join(FILES) + ') in internal/serve/ui/')
