#!/usr/bin/env python3
"""Synthetic preservation, conflict, lost-ack and binding regressions."""
import argparse
import importlib.util
import json
from pathlib import Path
import tempfile
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

    def test_inventory_limit_and_pagination(self):
        store = object.__new__(r.Store)
        store.request = lambda *a: {'rows': [{'id': 'a'}, {'id': 'b'}]}
        with self.assertRaises(r.ReplayError):
            store.inventory(1)
        with self.assertRaises(r.ReplayError):
            store.inventory(3)  # repeated/nonadvancing page

    def test_private_receipts_lost_ack_and_unchanged_baseline(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            local = root/'local.jsonl'
            local.write_text(json.dumps({'session': 'fixture', 'ts': '2026-01-01T00:00:00Z'})+'\n')
            source = {'original': {'id': 'original', 'session_id': 'fixture', 'ts': '2026-01-01T00:00:00Z', 'marks': '{"outcome":4}', 'mark_outcome': 4, 'findings': '[]', 'instance': 'fixture'}}
            unrelated = {'id': 'other', 'session_id': 'other'}
            schema = {k: {'type': 'int' if k == 'mark_outcome' else 'string', 'filterable': k not in ('marks', 'findings')} for k in source['original'] if k != 'id'}
            state = {'rows': {'other': unrelated.copy()}, 'lose_ack': True, 'writes': 0}
            class Fake:
                def __init__(self, config, endpoint, namespace): self.endpoint = endpoint
                def inventory(self, limit): return json.loads(json.dumps(source if self.endpoint == 'source' else state['rows']))
                def request(self, suffix='', body=None):
                    if body is None: return {'schema': schema}
                    if suffix == '/query': return {'rows': [{'id': 'original'}]}
                    assert body['upsert_condition'] == ['id', 'Eq', None]
                    state['writes'] += 1
                    for row in body['upsert_rows']: state['rows'].setdefault(row['id'], row)
                    if state['lose_ack']:
                        state['lose_ack'] = False
                        raise r.ReplayError('simulated lost ack')
                    return {}
            a = argparse.Namespace(receipts=root/'receipts', source_config=root/'source', target_config=root/'target', source_endpoint='source', target_endpoint='target', namespace='fixture-evals', jsonl=local, max_source_rows=10, max_target_rows=20, apply=True)
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
