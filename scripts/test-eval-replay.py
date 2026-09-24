#!/usr/bin/env python3
"""Exercise hev eval put against a disposable loopback HTTP contract store.

No real Layer service, embedding API or production namespace is contacted.
"""
import argparse
import datetime as dt
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--hev', required=True, type=Path)
    p.add_argument('--input', type=Path, help='Optional private adapted eval JSONL, replayed only into the disposable store')
    a = p.parse_args()
    stored, errors, batches = {}, [], []
    class Store(BaseHTTPRequestHandler):
        def log_message(self, *args): pass
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            if self.path != '/v2/namespaces/replay-fixture-evals' or body.get('upsert_condition') != ['id','Eq',None]:
                errors.append('namespace or insert-only condition missing')
                self.send_error(400)
                return
            rows = body['upsert_rows']
            batches.append(len(rows))
            for row in rows:
                stored.setdefault(row['id'], row)
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"status":"OK"}')
    with tempfile.TemporaryDirectory(prefix='eval-replay-') as tmp:
        root = Path(tmp)
        config = root/'config.toml'
        config.write_text('[layer]\napi_key = "fixture"\n')
        server = ThreadingHTTPServer(('127.0.0.1',0), Store)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        # Inherit no credentials, proxy or production endpoint configuration.
        env = {'PATH':os.environ['PATH'], 'HOME':str(root), 'HEV_CONFIG':str(config),
               'LAYER_ENDPOINT':f'http://127.0.0.1:{server.server_port}'}
        def put(rows, file=False):
            raw = ''.join(json.dumps(row)+'\n' for row in rows)
            args = [str(a.hev.resolve()),'eval','put','--namespace','replay-fixture']
            if file:
                path = root/'evals.jsonl'
                path.write_text(raw)
                args.append(str(path))
            return subprocess.run(args, input=None if file else raw, text=True, capture_output=True, env=env, timeout=60)
        try:
            rows = [dict(session=f'fixture-{i}',ts='2026-09-07T12:00:00Z',marks={'accuracy':4},
                         summary='Synthetic evaluation',evidence={'accuracy':'Fixture proof'},findings=[],
                         role='worker',instance='example',harness='codex',session_cost_usd=None,
                         eval_cost_usd=0,unknown=[],started='2026-09-07T11:00:00Z') for i in range(31)]
            first = put(rows, file=True)
            assert first.returncode == 0, first.stderr
            before = json.dumps(stored, sort_keys=True)
            for row in rows:
                row['ts']='2026-09-07T06:00:00-06:00'
                row['summary']='Must not overwrite original'
                row['marks']['accuracy']=1
            second = put(rows)
            assert second.returncode == 0, second.stderr
            assert len(stored)==31 and json.dumps(stored, sort_keys=True)==before, 'replay changed stored rows'
            expected = hashlib.sha256(b'fixture-0\0'+b'2026-09-07T12:00:00Z').hexdigest()[:32]
            assert expected in stored, 'identity is not canonical session/timestamp'
            rows[0]['ts']='2026-09-07T12:01:00Z'
            assert put(rows[:1]).returncode==0
            assert len(stored)==32, 'new evaluation timestamp lost'
            assert put([{'session':'invalid','ts':'bad','marks':{}}]).returncode!=0
            assert len(stored)==32 and not errors and max(batches)<=30
            if a.input:
                actual = [json.loads(line) for line in a.input.read_text().splitlines() if line.strip()]
                assert actual, 'empty input does not prove backfill'
                expected_ids = set()
                for row in actual:
                    timestamp = dt.datetime.fromisoformat(row['ts'].replace('Z','+00:00'))
                    assert timestamp.tzinfo is not None, 'timestamp lacks timezone'
                    # Preserve sub-microsecond precision rather than rounding it.
                    fraction = row['ts'].split('.',1)[1] if '.' in row['ts'] else ''
                    fraction = fraction.split('Z')[0].split('+')[0].split('-')[0].rstrip('0')
                    canonical = timestamp.astimezone(dt.timezone.utc).strftime('%Y-%m-%dT%H:%M:%S')
                    canonical += ('.'+fraction if fraction else '')+'Z'
                    expected_ids.add(hashlib.sha256((row['session']+'\0'+canonical).encode()).hexdigest()[:32])
                stored.clear()
                first = put(actual, file=True)
                assert first.returncode == 0, first.stderr
                before = json.dumps(stored, sort_keys=True)
                second = put(actual)
                assert second.returncode == 0, second.stderr
                assert set(stored)==expected_ids and json.dumps(stored, sort_keys=True)==before
                print(f'PASS: private input {len(actual)} rows, {len(stored)} unique eval IDs; second replay unchanged')
            print('PASS: public eval JSONL; file and stdin; 31 rows replay unchanged; equivalent UTC identity; new grade => 32; invalid row rejected; isolated HTTP contract store')
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

if __name__=='__main__': main()
