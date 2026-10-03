#!/usr/bin/env python3
"""Synthetic preservation, conflict, lost-ack and binding regressions."""
import argparse
import importlib.util
import json
from pathlib import Path
import tempfile
import contextlib
import io
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('replay', Path(__file__).with_name('replay-eval-corpus.py'))
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)


class ReplayTests(unittest.TestCase):
    def test_conflicts_and_schema(self):
        with self.assertRaises(r.ReplayError):
            r.reconcile({'a': {'id': 'a', 'marks': '1'}}, {}, {'a': {'id': 'a', 'marks': '2'}})
        with self.assertRaises(r.ReplayError):
            r.reconcile({}, {'b': {'id': 'b'}}, {})
        with self.assertRaises(r.ReplayError):
            r.check_schema({'ts': {'type': 'string', 'filterable': True}}, {'ts': {'type': 'string', 'filterable': False}})
        r.check_schema({'ts': {'type': 'string'}}, {'other': {'type': 'int'}})

    def test_projection_identity_findings_and_generated_vector(self):
        original = {'session': 'fixture', 'ts': '2026-01-01T06:00:00.123456789+06:00',
            'marks': {'outcome': 4}, 'findings': [{'summary': 'synthetic', 'turns': [1, 2]}],
            'evidence': {'outcome': 'synthetic'}, 'summary': 'synthetic'}
        row = r.project_local(original)
        self.assertEqual(row['ts'], original['ts'])
        self.assertEqual(r.canonical_ts(row['ts']), '2026-01-01T00:00:00.123456789Z')
        canonical = dict(original, ts='2026-01-01T00:00:00.123456789Z')
        self.assertEqual(row['id'], r.project_local(canonical)['id'])
        self.assertEqual(json.loads(json.loads(row['findings'])[0]), original['findings'][0])
        self.assertEqual(row['mark_outcome'], 4)
        with self.assertRaises(r.ReplayError): r.project_local(dict(original, marks={'outcome': 4.5}))
        with self.assertRaises(r.ReplayError): r.project_local(dict(original, marks={'outcome': True}))
        self.assertTrue(r.row_matches(row, dict(row, embed_text=[0.1]), ['embed_text']))
        self.assertFalse(r.row_matches(row, dict(row, summary='conflict', embed_text=[0.1]), ['embed_text']))
        self.assertFalse(r.row_matches(row, dict(row, unowned='unexpected'), ['embed_text']))

    def test_explicit_config_target_and_no_redirect(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = Path(tmp)/'config.toml'
            config.write_text('[layer]\nendpoint = "https://fixture.invalid"\napi_key = "fixture"\n')
            with self.assertRaises(r.ReplayError): r.Store(config, 'https://other.invalid', 'fixture-evals')
            self.assertEqual(r.Store(config, 'https://fixture.invalid', 'fixture-evals').endpoint, 'https://fixture.invalid')
            self.assertIsNone(r.NoRedirect().redirect_request(None, None, 302, None, {}, 'https://other.invalid'))

    def test_metadata_route_and_absence(self):
        store = object.__new__(r.Store)
        paths = []
        def request(path):
            paths.append(path)
            raise r.MissingNamespace('namespace absent')
        store.request = request
        self.assertIsNone(store.metadata(allow_missing=True))
        with self.assertRaises(r.MissingNamespace): store.metadata()
        self.assertEqual(paths, ['/metadata', '/metadata'])

    def test_inventory_limit_and_pagination(self):
        store = object.__new__(r.Store)
        store.request = lambda *a: {'rows': [{'id': 'a'}, {'id': 'b'}]}
        with self.assertRaises(r.ReplayError):
            store.inventory(1)
        with self.assertRaises(r.ReplayError):
            store.inventory(3)  # repeated/nonadvancing page

    def test_missing_destination_local_projection_and_conflict(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in ('source', 'target'): (root/name).write_text('fixture-'+name)
            first = {'session': 'stored', 'ts': '2026-01-01T00:00:00Z', 'marks': {'outcome': 4}, 'findings': []}
            second = dict(first, session='local', findings=[{'summary': 'fixture', 'turns': [1]}])
            local = root/'source.jsonl'
            local.write_text(''.join(json.dumps(e)+'\n' for e in [first, second]))
            stored = dict(r.project_local(first), id='original', embed_text=[0.25])
            schema = {k: {'type': 'int' if k == 'mark_outcome' else 'bool' if k == 'poor' else 'string',
                'filterable': k not in ('marks', 'evidence', 'findings', 'summary', 'text')} for k in stored if k not in ('id', 'embed_text')}
            schema['id'] = {'type': 'string'}
            schema['text']['embed'] = {'attribute': 'embed_text', 'model': 'fixture'}
            schema['embed_text'] = {'type': '[1]f16', 'filterable': False}
            current, created = {}, []
            class Fake:
                def __init__(self, config, endpoint, namespace): self.endpoint = endpoint
                def metadata(self, allow_missing=False):
                    if self.endpoint == 'target' and not current and allow_missing: return None
                    return schema
                def inventory(self, limit): return {'original': stored} if self.endpoint == 'source' else dict(current)
                def request(self, suffix='', body=None):
                    if suffix == '/query': return {'rows': [{'id': 'original'}]}
                    self.assertion(body)
                    count = 0
                    for row in body['upsert_rows']:
                        if row['id'] not in current:
                            current[row['id']] = dict(row, embed_text=row.get('embed_text', [0.5]))
                            created.append(row['id'])
                            count += 1
                    return {'rows_affected': count}
                @staticmethod
                def assertion(body): assert body['upsert_condition'] == ['id', 'Eq', None]
            a = argparse.Namespace(receipts=root/'receipts', source_config=root/'source', target_config=root/'target',
                source_endpoint='source', target_endpoint='target', namespace='fixture-evals', jsonl=local,
                max_source_rows=10, max_target_rows=10, expected_local_only=1, expected_stored_only=0,
                include_local_only=True, apply=True)
            with patch.object(r, 'Store', Fake), contextlib.redirect_stdout(io.StringIO()):
                r.run(a)
                r.run(a)
                self.assertEqual(len(created), 2)
                self.assertEqual(current['original'], stored)
                projected = r.project_local(second)
                self.assertEqual(json.loads(json.loads(current[projected['id']]['findings'])[0]), second['findings'][0])
                current[projected['id']]['mark_outcome'] = 1
                with self.assertRaises(r.ReplayError): r.run(a)
                with self.assertRaises(r.ReplayError): r.check_schema({'other': {'type': 'string'}}, {}, require_all=True)

    def test_private_receipts_lost_ack_and_unchanged_baseline(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root/'source').write_text('synthetic-source')
            (root/'target').write_text('synthetic-target')
            local = root/'local.jsonl'
            local_eval = {'session': 'fixture', 'ts': '2026-01-01T00:00:00Z', 'marks': {'outcome': 4}, 'instance': 'fixture', 'findings': []}
            local.write_text(json.dumps(local_eval)+'\n')
            source = {'original': dict(r.project_local(local_eval), id='original')}
            unrelated = {'id': 'other', 'session_id': 'other'}
            schema = {k: {'type': 'int' if k == 'mark_outcome' else 'bool' if k == 'poor' else 'string', 'filterable': k not in ('marks', 'findings', 'evidence', 'summary', 'text')} for k in source['original'] if k != 'id'}
            state = {'rows': {'other': unrelated.copy()}, 'lose_ack': True, 'writes': 0}
            class Fake:
                def __init__(self, config, endpoint, namespace): self.endpoint = endpoint
                def metadata(self, allow_missing=False):
                    if self.endpoint == 'target' and not state['rows']:
                        if allow_missing: return None
                        raise r.MissingNamespace('namespace absent')
                    return schema
                def inventory(self, limit): return json.loads(json.dumps(source if self.endpoint == 'source' else state['rows']))
                def request(self, suffix='', body=None):
                    if body is None: return {'schema': schema}
                    if suffix == '/query': return {'rows': [{'id': 'original'}]}
                    assert body['upsert_condition'] == ['id', 'Eq', None]
                    new = [row for row in body['upsert_rows'] if row['id'] not in state['rows']]
                    state['writes'] += bool(new)
                    for row in new: state['rows'].setdefault(row['id'], row)
                    if state['lose_ack']:
                        state['lose_ack'] = False
                        raise r.ReplayError('simulated lost ack')
                    return {'rows_upserted': len(new)}
            a = argparse.Namespace(receipts=root/'receipts', source_config=root/'source', target_config=root/'target', source_endpoint='source', target_endpoint='target', namespace='fixture-evals', jsonl=local, max_source_rows=10, max_target_rows=20, expected_local_only=0, expected_stored_only=0, include_local_only=False, apply=True)
            with patch.object(r, 'Store', Fake):
                with self.assertRaises(r.ReplayError): r.run(a)
                baseline = (a.receipts/'inventory.json').read_bytes()
                r.run(a)
                r.run(a)
                self.assertEqual(state['writes'], 1)
                self.assertEqual((a.receipts/'inventory.json').read_bytes(), baseline)
                self.assertEqual(state['rows']['original'], source['original'])
                self.assertEqual(state['rows']['other'], unrelated)
                state['rows']['other']['session_id'] = 'changed'
                with self.assertRaises(r.ReplayError): r.run(a)
                local.write_text(local.read_text()+'\n')
                with self.assertRaises(r.ReplayError): r.run(a)


if __name__ == '__main__': unittest.main()
