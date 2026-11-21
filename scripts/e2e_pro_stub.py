"""In-process stub of the minimal hev layer (Pro) /v1 + /v2 routes the kit CLI
calls, for scripts/test-e2e-oss-pro.py. Loopback only, no external network.

Routes:
  GET  /health                            liveness, no key
  GET  /v1/whoami                         who the bearer key is
  GET  /v1/namespaces/NS/schema           {} for a namespace with rows, else 404
  POST /v2/namespaces/NS                  upsert_rows / patch_rows / deletes
  POST /v2/namespaces/NS/query            rows (filters ignored, top_k honoured)
  POST /v1/search                         federated word match over every row

Every request is recorded (method, path, namespace, key) so a test can prove
which home wrote what. Unknown keys get 401; unknown routes 404.
"""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Stub:
    def __init__(self, keys):
        # keys: {bearer key: account name}
        self.keys = dict(keys)
        self.rows = {}  # namespace -> {id: row}
        self.requests = []
        self.lock = threading.Lock()
        stub = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def reply(self, code, body):
                raw = json.dumps(body).encode()
                self.send_response(code)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def handle_any(self, method):
                n = int(self.headers.get("Content-Length") or 0)
                body = json.loads(self.rfile.read(n) or b"{}") if n else {}
                auth = self.headers.get("Authorization", "")
                key = auth[len("Bearer "):] if auth.startswith("Bearer ") else ""
                path = self.path.split("?", 1)[0]
                parts = [p for p in path.split("/") if p]
                ns = parts[2] if len(parts) > 2 and parts[1] == "namespaces" else ""
                with stub.lock:
                    stub.requests.append({"method": method, "path": path, "namespace": ns, "account": stub.keys.get(key)})
                if path == "/health":
                    return self.reply(200, {"status": "ok"})
                if key not in stub.keys:
                    return self.reply(401, {"error": "unauthorized"})
                with stub.lock:
                    if path == "/v1/whoami":
                        return self.reply(200, {"account": stub.keys[key], "plan": "pro"})
                    if path == "/v1/search" and method == "POST":
                        words = str(body.get("query", "")).lower().split()
                        hits = []
                        for space, rows in stub.rows.items():
                            for row in rows.values():
                                text = json.dumps(row).lower()
                                if words and all(w in text for w in words):
                                    hits.append({"namespace": space, "id": row.get("id")})
                        return self.reply(200, {"hits": hits})
                    if ns and len(parts) == 5 and parts[4] == "schema" and method == "GET":
                        if ns in stub.rows:
                            return self.reply(200, {})
                        return self.reply(404, {"error": "namespace not found"})
                    if ns and len(parts) == 3 and method == "POST":
                        rows = stub.rows.setdefault(ns, {})
                        up = body.get("upsert_rows") or []
                        for row in up:
                            rows[str(row.get("id"))] = dict(row)
                        for row in body.get("patch_rows") or []:
                            rows.setdefault(str(row.get("id")), {}).update(row)
                        for rid in body.get("deletes") or []:
                            rows.pop(str(rid), None)
                        return self.reply(200, {"status": "OK", "rows_upserted": len(up)})
                    if ns and len(parts) == 4 and parts[3] == "query" and method == "POST":
                        if ns not in stub.rows:
                            return self.reply(404, {"error": "namespace not found"})
                        rows = list(stub.rows[ns].values())
                        top = body.get("top_k") or len(rows)
                        return self.reply(200, {"rows": rows[: int(top)]})
                return self.reply(404, {"error": "not found"})

            def do_GET(self):
                self.handle_any("GET")

            def do_POST(self):
                self.handle_any("POST")

            def do_PATCH(self):
                self.handle_any("PATCH")

            def do_DELETE(self):
                self.handle_any("DELETE")

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = "http://127.0.0.1:%d" % self.server.server_address[1]
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *exc):
        self.server.shutdown()
        self.server.server_close()

    def writes(self, account=None):
        with self.lock:
            return [r for r in self.requests if r["method"] == "POST" and r["namespace"] and not r["path"].endswith("/query")
                    and (account is None or r["account"] == account)]

    def namespaces(self):
        with self.lock:
            return {ns: len(rows) for ns, rows in self.rows.items()}
