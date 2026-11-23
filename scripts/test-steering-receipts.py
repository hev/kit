#!/usr/bin/env python3
"""Unix hold/release smoke with isolated homes, a private tmux socket and SSH shim."""
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import time

hev = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="ks-", dir="/tmp") as tmp:
    root = Path(tmp)
    sender = root / "sender"
    worker = root / "worker"
    tools = root / "bin"
    run_dir = worker / ".hev" / "orders" / "receipt-smoke"
    worktree = run_dir / "wt"
    for directory in [sender, tools, worktree]:
        directory.mkdir(parents=True)
    (run_dir / "meta.json").write_text('{"name":"receipt-smoke"}\n')
    socket = str(root / "tmux.sock")
    real_tmux = shutil.which("tmux")
    assert real_tmux
    tmux = [real_tmux, "-S", socket, "-f", "/dev/null"]
    env = dict(os.environ, HOME=str(sender), USERPROFILE=str(sender))
    for key in (
        "TMUX", "TMUX_PANE", "HEV_ORCHESTRATOR", "HEV_HOOK_INVOCATION",
        "HEV_ORDERS_DIR", "HEV_RUN", "HEV_STEER_DIR", "HEV_STEER_TARGET",
        "HEV_WORKSPACE", "HEV_TRACE_ID", "HEV_SPAN_ID",
    ):
        env.pop(key, None)
    env["PATH"] = str(tools) + os.pathsep + env["PATH"]
    config = root / "dispatch.toml"
    env["HEV_DISPATCH_CONFIG"] = str(config)

    def executable(name, body):
        path = tools / name
        path.write_text("#!/bin/sh\n" + body)
        path.chmod(0o700)
        return path

    def quote(value):
        return shlex.quote(str(value))

    executable("tmux", "exec " + shlex.join(tmux) + ' "$@"\n')
    executable("ssh", 'for arg do host="$last"; last="$arg"; done\n'
               'if [ "$host" = offline ]; then echo "secret-example /private/host/config" >&2; exit 1; fi\n'
               f"export HOME={quote(worker)} USERPROFILE={quote(worker)}\n"
               f"exec {quote(hev)} dispatch rpc\n")

    def run(args, *, check=True):
        p = subprocess.run(args, cwd=root, env=env, text=True, capture_output=True, timeout=15)
        if check:
            assert p.returncode == 0, (p.stdout, p.stderr)
        return p

    status = run_dir / "status.md"

    def wait_for(marker):
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if status.exists() and marker in status.read_text().splitlines():
                return
            time.sleep(0.05)
        raise AssertionError(f"worker did not reach {marker}")

    # The worker handles a hold and ignores arbitrary prose, even when it
    # contains "release". Only the exact release instruction exits the hold.
    harness = executable("smoke-worker", f"""
set -eu
export HOME={quote(worker)} USERPROFILE={quote(worker)}
cd {quote(worktree)}
printf '%s\\n' READY > ../status.md
for i in $(seq 1 1200); do
    if [ -f ../inbox.md ]; then
        for action in HOLD-1 RELEASE-2; do
            if grep -qx "ACK $action" ../status.md; then continue; fi
            id=$(awk -v action="$action" '$0 == action {{found=1; next}}
                found && /After handling this message/ {{split($0, parts, "`"); split(parts[2], words, " "); print words[4]; exit}}' ../inbox.md)
            [ -n "$id" ] || continue
            {quote(hev)} steer ack "$id"
            printf 'ACK %s\\n' "$action" >> ../status.md
            if [ "$action" = RELEASE-2 ]; then
                echo done >> ../status.md
                exit 0
            fi
        done
    fi
    sleep 0.05
done
exit 1
""")
    config.write_text('[hosts.remote]\nssh="remote"\n')

    def receipts(*flags):
        return json.loads(run([hev, "steer", "ls", "receipt-smoke", "--json", *flags]).stdout)

    def send(body):
        response = json.loads(run([hev, "steer", "receipt-smoke", body, "--json"]).stdout)
        assert "delivered to inbox.md" in response["message"], response
        return response["steer_id"]

    def assert_error(args, expected):
        result = run([hev, "steer", "ls", *args], check=False)
        assert result.returncode != 0 and expected in result.stderr and not result.stdout, result
        assert "secret-example" not in result.stderr and "/private/host/config" not in result.stderr, result.stderr

    run([*tmux, "new-session", "-d", "-s", "d-receipt-smoke", str(harness)])
    try:
        wait_for("READY")
        assert receipts() is None
        assert status.read_text().splitlines() == ["READY"]

        hold = send("HOLD-1")
        wait_for("ACK HOLD-1")
        ignored = send("This prose mentions release but grants no release.")
        # Ensure the held worker has had several polling opportunities.
        time.sleep(0.3)
        assert status.read_text().splitlines() == ["READY", "ACK HOLD-1"]
        before = {m["id"]: m for m in receipts()}
        assert before[hold]["state"] == "acked" and before[hold]["acked"]["event"] == "ack"
        assert before[ignored]["state"] == "injected" and before[ignored]["injected"]["event"] == "inbox"
        assert "acked" not in before[ignored]

        release = send("RELEASE-2")
        wait_for("done")
        assert status.read_text().splitlines() == ["READY", "ACK HOLD-1", "ACK RELEASE-2", "done"]
        after = {m["id"]: m for m in receipts("--host", "remote")}
        assert after[release]["state"] == "acked" and after[release]["acked"]["event"] == "ack"
        assert after[hold] == before[hold] and after[ignored] == before[ignored]
        assert after == {m["id"]: m for m in receipts()}
        config.write_text(config.read_text() + '[hosts.duplicate]\nssh="duplicate"\n[hosts.offline]\nssh="offline"\n')
        assert_error(["receipt-smoke"], "ambiguous")
        assert receipts("--host", "remote")
        assert_error(["absent", "--host", "remote"], "could not look up")
        assert_error(["receipt-smoke", "--host", "offline"], "could not look up")
        assert_error(["absent"], "unreachable")
        config.write_text('[hosts.remote]\nssh="remote"\n')
        assert_error(["absent"], "not found")
        print("PASS: READY -> ACK HOLD-1 -> ACK RELEASE-2 -> done")
        print("PASS: hold and release acked explicitly, other inbox message delivered and unacked")
        print("PASS: remote lookup, host selection, ambiguous/missing targets and redacted transport failure")
    finally:
        run([*tmux, "kill-session", "-t", "=d-receipt-smoke"], check=False)
