#!/usr/bin/env python3
"""Probe a Kit route without requesting an archive scan or printing row data."""
import argparse
import hashlib
import json
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def probe(base):
    url = urllib.parse.urlsplit(base)
    if url.scheme not in ("http", "https") or not url.netloc or url.username or url.password:
        raise ValueError("expected an HTTP(S) origin without credentials")
    if url.path not in ("", "/") or url.query or url.fragment:
        raise ValueError("expected an origin without path, query or fragment")
    opener = urllib.request.build_opener(NoRedirect())
    observations = []
    # Invalid windows are rejected before accessing the reader. Do not use a
    # valid window, even with limit=1: older servers ignore pagination limits.
    for path, expected in (("/", 200), ("/api/sessions?window=preflight-invalid", 400),
                           ("/api/stats?window=preflight-invalid", 400)):
        req = urllib.request.Request(base.rstrip("/") + path,
                                     headers={"User-Agent": "kit-readiness/1"})
        try:
            response = opener.open(req, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read(2 * 1024 * 1024 + 1)
            ok = response.code == expected and len(body) <= 2 * 1024 * 1024
            if path == "/":
                ok = ok and b"/api/sessions" in body and b"/api/stats" in body
            else:
                ok = ok and b"window" in body.lower()
            observations.append({"path": path, "http": response.code,
                                 "expected": expected, "passed": ok,
                                 "bytes": len(body), "sha256": hashlib.sha256(body).hexdigest()})
    return {"ready": all(row["passed"] for row in observations),
            "scope": "dashboard route only; no archive, auth or backend parity acceptance",
            "checks": observations}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("origin")
    args = parser.parse_args()
    try:
        result = probe(args.origin)
    except Exception as error:
        # Exception URLs/bodies may contain sensitive application details.
        result = {"ready": False, "error": type(error).__name__}
    print(json.dumps(result, sort_keys=True))
    raise SystemExit(0 if result["ready"] else 1)
