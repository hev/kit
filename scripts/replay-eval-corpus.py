#!/usr/bin/env python3
"""Bounded, insert-only migration of stored eval rows. Receipts are private.

Python 3.11+. Explicit config files supply credentials; no environment fallback.
No evaluator prose, IDs, credentials or response bodies are printed.
"""
import argparse
import fcntl
import datetime as dt
import re
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


class MissingNamespace(ReplayError):
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
            if e.code == 404:
                raise MissingNamespace('namespace absent') from None
            raise ReplayError('store HTTP '+str(e.code)) from None
        if result.get('error'):
            raise ReplayError('store returned an error (body suppressed)')
        return result

    def metadata(self, allow_missing=False):
        try:
            return self.request('/metadata')['schema']
        except MissingNamespace:
            if allow_missing:
                return None
            raise

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


def check_schema(source, target, require_all=False):
    # Additional target fields belong to other writers. Never remove them.
    for key, value in source.items():
        if (require_all and key not in target) or (key in target and target[key] != value):
            raise ReplayError('source/target schema conflict')


def row_matches(expected, actual, generated_fields=()):
    if actual is None:
        return False
    actual = {k: v for k, v in actual.items() if k not in generated_fields or k in expected}
    return expected == actual


def reconcile(source, baseline, current, projected_ids=(), generated_fields=()):
    for ident, row in baseline.items():
        if current.get(ident) != row:
            raise ReplayError('preexisting target row changed or disappeared')
    for ident, row in source.items():
        if ident in current and not row_matches(row, current[ident], generated_fields if ident in projected_ids else ()):
            raise ReplayError('source ID conflicts with target row')
    return [row for ident, row in source.items() if ident not in current]


def canonical_ts(value):
    match = re.fullmatch(r'(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})', value)
    if not match:
        raise ReplayError('eval timestamp must be RFC3339 with nanosecond precision or less')
    try:
        stamp = dt.datetime.fromisoformat(match[1]+match[3].replace('Z', '+00:00')).astimezone(dt.timezone.utc)
    except ValueError:
        raise ReplayError('invalid eval timestamp') from None
    fraction = (match[2] or '').rstrip('0')
    base = f'{stamp.year:04d}-{stamp.month:02d}-{stamp.day:02d}T{stamp.hour:02d}:{stamp.minute:02d}:{stamp.second:02d}'
    return base+('.'+fraction if fraction else '')+'Z'


def project_local(e):
    # Supported Eval wire shape for local-only rows. Preserve source timestamp;
    # only identity uses canonical UTC. Structured findings become lossless JSON
    # strings because trace.Eval requires []string. Never regenerate stored rows.
    session, ts = e.get('session'), e.get('ts')
    if not isinstance(session, str) or not session.strip() or not isinstance(ts, str):
        raise ReplayError('eval session and timestamp required')
    marks = e.get('marks')
    evidence = e.get('evidence') if e.get('evidence') is not None else {}
    findings = e.get('findings') if e.get('findings') is not None else []
    if not isinstance(marks, dict) or not isinstance(evidence, dict) or not isinstance(findings, list):
        raise ReplayError('invalid eval marks, evidence or findings shape')
    if any(not isinstance(k, str) or not re.fullmatch(r'[A-Za-z][A-Za-z0-9_]*', k) or type(v) is not int or not -(2**63) <= v < 2**63 for k, v in marks.items()):
        raise ReplayError('eval marks require supported names and int64 values')
    if any(not isinstance(k, str) or not isinstance(v, str) for k, v in evidence.items()):
        raise ReplayError('eval evidence must contain strings')
    findings = [v if isinstance(v, str) else json.dumps(v, sort_keys=True, separators=(',', ':'), ensure_ascii=False, allow_nan=False) for v in findings]
    row = {'id': hashlib.sha256((session+'\0'+canonical_ts(ts)).encode()).hexdigest()[:32],
        'session_id': session, 'ts': ts, 'poor': e.get('poor', False)}
    if type(row['poor']) is not bool:
        raise ReplayError('eval poor must be boolean')
    for key in ('role', 'instance', 'host', 'summary'):
        row[key] = e.get(key, '')
        if not isinstance(row[key], str):
            raise ReplayError('eval scalar attributes must be strings')
    for key, value in [('marks', marks), ('evidence', evidence), ('findings', findings)]:
        row[key] = json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False)
    row.update({'mark_'+key: value for key, value in marks.items()})
    row['text'] = '\n'.join([row['summary'], *[key+': '+evidence[key] for key in sorted(evidence)], *findings])
    return row


def run(a):
    receipts = a.receipts.resolve()
    for parent in [receipts, *receipts.parents]:
        if (parent/'.git').exists():
            raise ReplayError('receipts must be outside a git checkout')
    receipts.mkdir(mode=0o700, parents=True, exist_ok=True)
    if receipts.stat().st_mode & 0o077:
        raise ReplayError('receipt directory must have mode 0700')
    with (receipts/'replay.lock').open('a') as lock:
        os.chmod(receipts/'replay.lock', 0o600)
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ReplayError('another replay owns this receipt directory') from None
        return run_locked(a)


def run_locked(a):
    receipts = a.receipts.resolve()
    source = Store(a.source_config, a.source_endpoint, a.namespace)
    target = Store(a.target_config, a.target_endpoint, a.namespace)
    if source.endpoint == target.endpoint:
        raise ReplayError('source and target must differ')
    binding = {'source_endpoint': source.endpoint, 'target_endpoint': target.endpoint,
        'namespace': a.namespace, 'max_source_rows': a.max_source_rows,
        'max_target_rows': a.max_target_rows, 'source_config_sha256': hashlib.sha256(a.source_config.read_bytes()).hexdigest(),
        'target_config_sha256': hashlib.sha256(a.target_config.read_bytes()).hexdigest(),
        'include_local_only': a.include_local_only, 'plan_version': 2,
        'expected_local_only': a.expected_local_only, 'expected_stored_only': a.expected_stored_only,
        'jsonl_sha256': hashlib.sha256(a.jsonl.read_bytes()).hexdigest()}
    journal = receipts/'inventory.json'
    if journal.exists():
        state = json.loads(journal.read_text())
        if state['binding'] != binding:
            raise ReplayError('receipt binding changed; do not replace preservation baseline')
    else:
        schema = source.metadata()
        target_schema = target.metadata(allow_missing=True)
        check_schema(schema, target_schema or {})
        rows = source.inventory(a.max_source_rows)
        baseline = target.inventory(a.max_target_rows) if target_schema is not None else {}
        if not rows:
            raise ReplayError('empty source cannot prove migration')
        # Old stored rows are authoritative for wire encoding and original IDs.
        # JSONL coverage binds the bounded local corpus without recoding timestamps.
        local = [json.loads(line) for line in a.jsonl.read_text().splitlines() if line.strip()]
        if len(local) > a.max_source_rows:
            raise ReplayError('local corpus exceeded explicit row bound')
        keys = {(row.get('session_id'), row.get('ts')) for row in rows.values()}
        local_keys = {(row.get('session'), row.get('ts')) for row in local}
        local_only, stored_only = len(local_keys-keys), len(keys-local_keys)
        if not local_keys or (local_only, stored_only) != (a.expected_local_only, a.expected_stored_only):
            raise ReplayError('corpus identity differences do not match reviewed explicit bounds')
        old_by_key = {}
        for row in rows.values():
            old_by_key.setdefault((row.get('session_id'), row.get('ts')), []).append(row)
        historical_count = len(rows)
        projected_ids = set()
        if local_only and not a.include_local_only:
            raise ReplayError('local-only evaluations require explicit supported projection')
        for e in local:
            key = (e.get('session'), e.get('ts'))
            row = project_local(e)
            if key in keys:
                for original in old_by_key[key]:
                    for field in ('instance', 'host', 'role', 'poor', 'summary', 'marks', 'evidence'):
                        actual, expected = original.get(field), row[field]
                        if field in ('marks', 'evidence'):
                            actual, expected = json.loads(actual), json.loads(expected)
                        if actual != expected:
                            raise ReplayError('shared source identity has conflicting evaluation values')
                    if all(isinstance(v, str) for v in e.get('findings', [])):
                        if json.loads(original['findings']) != json.loads(row['findings']):
                            raise ReplayError('shared source findings conflict with stored findings')
                    for name, value in e['marks'].items():
                        if original.get('mark_'+name) != value:
                            raise ReplayError('shared source filterable mark differs from stored mark')
                continue  # Never replace historical findings encoding or stored vectors.

            for field in row:
                if field not in schema:
                    raise ReplayError('projected local field absent from original schema')
            for name in e['marks']:
                if schema['mark_'+name] != {'type': 'int', 'filterable': True}:
                    raise ReplayError('local mark schema conflicts with original schema')
            if row['id'] in rows and rows[row['id']] != row:
                raise ReplayError('local identity conflicts with original stored or duplicate row')
            rows[row['id']] = row
            projected_ids.add(row['id'])
        generated_fields = {v['embed']['attribute'] for v in schema.values() if 'embed' in v}
        reconcile(rows, baseline, baseline, projected_ids, generated_fields)
        state = {'binding': binding, 'source_schema': schema, 'target_schema': target_schema or {},
            'projected_ids': sorted(projected_ids), 'generated_fields': sorted(generated_fields),
            'target_absent': target_schema is None, 'source': rows, 'baseline': baseline,
            'local_rows': len(local), 'local_unique': len(local_keys), 'local_only_unique': local_only,
            'stored_only_unique': stored_only, 'historical_rows': historical_count}
        save(journal, state)
    rows, baseline, schema = state['source'], state['baseline'], state['source_schema']
    current_schema = target.metadata(allow_missing=True)
    check_schema(schema, current_schema or {})
    # Preserve the entire preflight schema, including unrelated target definitions.
    check_schema(state['target_schema'], current_schema or {}, require_all=True)
    if current_schema is None and baseline:
        raise ReplayError('preexisting target namespace disappeared')
    current = target.inventory(a.max_target_rows) if current_schema is not None else {}
    missing = reconcile(rows, baseline, current, state['projected_ids'], state['generated_fields'])
    if len(set(rows) | set(current)) > a.max_target_rows:
        raise ReplayError('planned target inventory exceeds explicit row bound')
    print(json.dumps({'phase': 'preflight', 'local_rows': state['local_rows'],
        'local_unique': state['local_unique'], 'local_only_unique': state['local_only_unique'], 'source_rows': len(rows), 'target_rows': len(current),
        'historical_rows': state['historical_rows'], 'stored_only_unique': state['stored_only_unique'], 'missing_rows': len(missing), 'apply': a.apply}))
    if not a.apply:
        return
    for start in range(0, len(missing), 30):
        target.request(body={'upsert_rows': missing[start:start+30], 'schema': schema,
            'upsert_condition': ['id', 'Eq', None], 'distance_metric': 'cosine_distance'})
    after = target.inventory(a.max_target_rows)
    if reconcile(rows, baseline, after, state['projected_ids'], state['generated_fields']):
        raise ReplayError('readback missing source rows')
    after_schema = target.metadata()
    check_schema(schema, after_schema)
    check_schema(state['target_schema'], after_schema, require_all=True)
    if any(after_schema.get(k) != v for k, v in schema.items()):
        raise ReplayError('source schema did not round trip exactly')
    # A same-value existing-row probe cannot damage data even if a provider
    # ignores the condition. Require its affected-row counter to prove refusal.
    probe_row = after[next(iter(rows))]
    refused = target.request(body={'upsert_rows': [probe_row], 'schema': schema,
        'upsert_condition': ['id', 'Eq', None], 'distance_metric': 'cosine_distance'})
    affected = refused.get('rows_affected', refused.get('rows_upserted'))
    if type(affected) is not int or affected != 0:
        raise ReplayError('existing-row probe did not prove insert-only refusal')
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
        'local_unique': state['local_unique'], 'local_only_unique': state['local_only_unique'], 'target_rows': len(after),
        'preserved_baseline_rows': len(baseline), 'filter_probes': probes, 'insert_only_probe_affected': affected,
        'source_digest': digest(rows), 'baseline_digest': digest(baseline),
        'historical_rows': state['historical_rows'], 'stored_only_unique': state['stored_only_unique'], 'schema_digest': digest(schema), 'snapshot': False}
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
    p.add_argument('--expected-local-only', type=int, default=0, help='Reviewed local identities outside the old stored cohort')
    p.add_argument('--expected-stored-only', type=int, default=0, help='Reviewed old stored identities absent locally; retained as-is')
    p.add_argument('--include-local-only', action='store_true', help='Project reviewed local-only rows through the supported Eval wire shape')
    p.add_argument('--apply', action='store_true')
    a = p.parse_args()
    if min(a.max_source_rows, a.max_target_rows) <= 0 or min(a.expected_local_only, a.expected_stored_only) < 0 or '/' in a.namespace:
        p.error('positive bounds and a namespace without slashes required')
    try:
        run(a)
    except (ValueError, KeyError, TypeError, AttributeError, OSError) as e:
        # Avoid printing response contents, paths or invalid private values.
        print('Replay stopped: '+str(e) if isinstance(e, ReplayError) else 'Replay stopped: configuration or transport failure')
        raise SystemExit(1)


if __name__ == '__main__':
    main()
