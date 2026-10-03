#!/usr/bin/env python3
"""Bounded, insert-only migration of stored eval rows. Receipts are private.

Python 3.11+. Explicit config files supply credentials; no environment fallback.
No evaluator prose, IDs, credentials or response bodies are printed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import tempfile
import tomllib
import urllib.error
import urllib.request


class ReplayError(ValueError):
    pass


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()).hexdigest()


def save(path, value):
    with tempfile.NamedTemporaryFile(mode='w', dir=path.parent, delete=False) as f:
        os.chmod(f.name, 0o600)
        json.dump(value, f, ensure_ascii=False)
        f.flush()
        os.fsync(f.fileno())
    os.replace(f.name, path)


class Store:
    def __init__(self, config, endpoint, namespace):
        cfg = tomllib.loads(config.read_text())['layer']
        if not cfg.get('api_key'):
            raise ReplayError('explicit config has no API key')
        if endpoint != cfg.get('endpoint'):
            raise ReplayError('explicit endpoint does not match config')
        if not endpoint.startswith('https://'):
            raise ReplayError('HTTPS endpoint required')
        self.endpoint, self.key, self.namespace = endpoint.rstrip('/'), cfg['api_key'], namespace

    def request(self, suffix='', body=None):
        req = urllib.request.Request(self.endpoint+'/v2/namespaces/'+self.namespace+suffix,
            data=None if body is None else json.dumps(body).encode(), headers={
                'Authorization': 'Bearer '+self.key, 'Content-Type': 'application/json',
                'User-Agent': 'hevlayer-backfill/1.0'})
        try:
            with urllib.request.urlopen(req, timeout=120) as response:
                result = json.load(response)
        except urllib.error.HTTPError as e:
            raise ReplayError('store HTTP '+str(e.code)) from None
        if result.get('error'):
            raise ReplayError('store returned an error (body suppressed)')
        return result

    def inventory(self, limit):
        rows, cursor = {}, None
        while True:
            body = {'rank_by': ['id', 'asc'], 'top_k': min(1000, limit+1-len(rows)), 'include_attributes': True}
            if cursor is not None:
                body['filters'] = ['id', 'Gt', cursor]
            page = self.request('/query', body).get('rows', [])
            if not page:
                return rows
            for row in page:
                row = dict(row)
                row.pop('$dist', None)
                ident = row.get('id')
                if not isinstance(ident, str) or ident in rows or (cursor is not None and ident <= cursor):
                    raise ReplayError('duplicate, invalid or nonadvancing inventory ID')
                if any(k.startswith('$') for k in row):
                    raise ReplayError('unknown response-only attribute')
                rows[ident] = row
            if len(rows) > limit:
                raise ReplayError('inventory exceeded explicit row bound')
            cursor = page[-1]['id']


def check_schema(source, target):
    # Additional target fields belong to other writers. Never remove them.
    for key, value in source.items():
        if key in target and target[key] != value:
            raise ReplayError('source/target schema conflict')


def reconcile(source, baseline, current):
    for ident, row in baseline.items():
        if current.get(ident) != row:
            raise ReplayError('preexisting target row changed or disappeared')
    for ident, row in source.items():
        if ident in current and current[ident] != row:
            raise ReplayError('source ID conflicts with target row')
    return [row for ident, row in source.items() if ident not in current]


def run(a):
    # A private receipt directory must not be placed in any git checkout.
    receipts = a.receipts.resolve()
    for parent in [receipts, *receipts.parents]:
        if (parent/'.git').exists():
            raise ReplayError('receipts must be outside a git checkout')
    receipts.mkdir(mode=0o700, parents=True, exist_ok=True)
    if receipts.stat().st_mode & 0o077:
        raise ReplayError('receipt directory must have mode 0700')
    source = Store(a.source_config, a.source_endpoint, a.namespace)
    target = Store(a.target_config, a.target_endpoint, a.namespace)
    if source.endpoint == target.endpoint:
        raise ReplayError('source and target must differ')
    binding = {'source_endpoint': source.endpoint, 'target_endpoint': target.endpoint,
        'namespace': a.namespace, 'max_source_rows': a.max_source_rows,
        'max_target_rows': a.max_target_rows, 'jsonl_sha256': hashlib.sha256(a.jsonl.read_bytes()).hexdigest()}
    journal = receipts/'inventory.json'
    if journal.exists():
        state = json.loads(journal.read_text())
        if state['binding'] != binding:
            raise ReplayError('receipt binding changed; do not replace preservation baseline')
    else:
        schema = source.request()['schema']
        target_schema = target.request()['schema']
        check_schema(schema, target_schema)
        rows = source.inventory(a.max_source_rows)
        baseline = target.inventory(a.max_target_rows)
        if not rows:
            raise ReplayError('empty source cannot prove migration')
        # Old stored rows are authoritative for wire encoding and original IDs.
        # JSONL coverage binds the bounded local corpus without recoding timestamps.
        local = [json.loads(line) for line in a.jsonl.read_text().splitlines() if line.strip()]
        keys = {(row.get('session_id'), row.get('ts')) for row in rows.values()}
        local_keys = {(row.get('session'), row.get('ts')) for row in local}
        if not local_keys or not local_keys.issubset(keys):
            raise ReplayError('local corpus identities not fully covered by old inventory; adaptation requires review')
        reconcile(rows, baseline, baseline)
        state = {'binding': binding, 'source_schema': schema, 'target_schema': target_schema,
            'source': rows, 'baseline': baseline, 'local_rows': len(local), 'local_unique': len(local_keys)}
        save(journal, state)
    rows, baseline, schema = state['source'], state['baseline'], state['source_schema']
    current_schema = target.request()['schema']
    check_schema(schema, current_schema)
    # Preserve the entire preflight schema, including unrelated target definitions.
    check_schema(state['target_schema'], current_schema)
    current = target.inventory(a.max_target_rows)
    missing = reconcile(rows, baseline, current)
    print(json.dumps({'phase': 'preflight', 'local_rows': state['local_rows'],
        'local_unique': state['local_unique'], 'source_rows': len(rows), 'target_rows': len(current),
        'missing_rows': len(missing), 'apply': a.apply}))
    if not a.apply:
        return
    for start in range(0, len(missing), 30):
        target.request(body={'upsert_rows': missing[start:start+30], 'schema': schema,
            'upsert_condition': ['id', 'Eq', None], 'distance_metric': 'cosine_distance'})
    after = target.inventory(a.max_target_rows)
    if reconcile(rows, baseline, after):
        raise ReplayError('readback missing source rows')
    after_schema = target.request()['schema']
    check_schema(schema, after_schema)
    check_schema(state['target_schema'], after_schema)
    if any(after_schema.get(k) != v for k, v in schema.items()):
        raise ReplayError('source schema did not round trip exactly')
    # Exercise each source filterable attribute using an actual non-null value.
    probes = 0
    for field, definition in schema.items():
        if definition.get('filterable', True) and field != 'vector':
            sample = next((row for row in rows.values() if row.get(field) is not None), None)
            if sample is None:
                raise ReplayError('filterable source field has no value to probe')
            value = sample[field]
            op = 'ContainsAny' if isinstance(value, list) else 'Eq'
            probe = target.request('/query', {'rank_by': ['id', 'asc'], 'top_k': 1,
                'include_attributes': False, 'filters': ['And', [['id', 'Eq', sample['id']], [field, op, value]]]})
            if not probe.get('rows'):
                raise ReplayError('filterability probe returned no hits')
            probes += 1
    save(receipts/'readback.json', {'schema': after_schema, 'rows': after})
    summary = {'phase': 'verified', 'source_rows': len(rows), 'local_rows': state['local_rows'],
        'local_unique': state['local_unique'], 'target_rows': len(after),
        'preserved_baseline_rows': len(baseline), 'filter_probes': probes,
        'source_digest': digest(rows), 'baseline_digest': digest(baseline),
        'schema_digest': digest(schema), 'snapshot': False}
    save(receipts/'summary.json', summary)
    print(json.dumps(summary))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ('source-config', 'target-config', 'jsonl', 'receipts'):
        p.add_argument('--'+name, type=Path, required=True)
    for name in ('source-endpoint', 'target-endpoint', 'namespace'):
        p.add_argument('--'+name, required=True)
    p.add_argument('--max-source-rows', type=int, required=True)
    p.add_argument('--max-target-rows', type=int, required=True)
    p.add_argument('--apply', action='store_true')
    a = p.parse_args()
    if min(a.max_source_rows, a.max_target_rows) <= 0 or '/' in a.namespace:
        p.error('positive bounds and a namespace without slashes required')
    try:
        run(a)
    except (ValueError, KeyError, OSError) as e:
        # Avoid printing response contents, paths or invalid private values.
        print('Replay stopped: '+str(e) if isinstance(e, ReplayError) else 'Replay stopped: configuration or transport failure')
        raise SystemExit(1)


if __name__ == '__main__':
    main()
