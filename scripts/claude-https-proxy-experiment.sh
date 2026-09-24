#!/usr/bin/env bash
set -euo pipefail

PORT="${HEV_PROXY_PORT:-8088}"
CAPTURE_DIR="${HEV_PROXY_CAPTURE_DIR:-$HOME/.hev/claude-code/https-proxy}"
ADDON="$CAPTURE_DIR/mitm-claude-capture.py"

if ! command -v mitmdump >/dev/null 2>&1; then
    cat >&2 <<MSG
mitmdump is required for the HTTPS proxy experiment.

Install it, then rerun:
  brew install mitmproxy
MSG
    exit 1
fi

mkdir -p "$CAPTURE_DIR/bodies"

cat > "$ADDON" <<'PY'
from __future__ import annotations

import json
import time
import uuid
from pathlib import Path

from mitmproxy import http


ROOT = Path(__file__).resolve().parent
BODIES = ROOT / "bodies"
INDEX = ROOT / "flows.jsonl"


def interesting(host: str) -> bool:
    host = host.lower()
    return host.endswith("anthropic.com") or host.endswith("claude.ai")


def write_bytes(name: str, data: bytes) -> str | None:
    if not data:
        return None
    path = BODIES / name
    path.write_bytes(data)
    return str(path)


def response(flow: http.HTTPFlow) -> None:
    if not interesting(flow.request.host):
        return

    flow_id = str(uuid.uuid4())
    req_ref = write_bytes(f"{flow_id}.request.bin", flow.request.raw_content or b"")
    resp_ref = write_bytes(f"{flow_id}.response.bin", flow.response.raw_content or b"")

    record = {
        "id": flow_id,
        "ts": time.time(),
        "method": flow.request.method,
        "url": flow.request.pretty_url,
        "scheme": flow.request.scheme,
        "host": flow.request.host,
        "path": flow.request.path,
        "status_code": flow.response.status_code if flow.response else None,
        "request_headers": dict(flow.request.headers),
        "response_headers": dict(flow.response.headers) if flow.response else {},
        "request_body_ref": req_ref,
        "response_body_ref": resp_ref,
        "request_size": len(flow.request.raw_content or b""),
        "response_size": len(flow.response.raw_content or b"") if flow.response else 0,
    }
    with INDEX.open("a", encoding="utf-8") as f:
        f.write(json.dumps(record, separators=(",", ":"), sort_keys=True))
        f.write("\n")
PY

cat <<MSG
Starting mitmproxy on http://127.0.0.1:$PORT
Capture directory: $CAPTURE_DIR

In another shell, run Claude Code with:
  export HTTPS_PROXY=http://127.0.0.1:$PORT
  export HTTP_PROXY=http://127.0.0.1:$PORT
  export NODE_EXTRA_CA_CERTS="$HOME/.mitmproxy/mitmproxy-ca-cert.pem"
  export CLAUDE_CODE_CERT_STORE=bundled,system
  claude

If the CA is not trusted, this will only prove proxy routing; it will not expose
request/response bodies.
MSG

exec mitmdump --listen-host 127.0.0.1 --listen-port "$PORT" -s "$ADDON"
