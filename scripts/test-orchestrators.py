#!/usr/bin/env python3
"""Deterministic CLI acceptance with a fake source and harness, on a private tmux socket.

Usage: go build -o /tmp/hev-test ./cmd/hev && python3 scripts/test-orchestrators.py /tmp/hev-test
No network, model, operator tmux server, or operator state is used.
"""
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import time


def main():
    hev = str(Path(sys.argv[1]).resolve())
    real_tmux = shutil.which("tmux")
    assert real_tmux, "tmux is required"
    fixture = Path(__file__).resolve().parents[1] / "internal/dispatch/testdata/source/hev-fake-source"
    with tempfile.TemporaryDirectory(prefix="orch-", dir="/tmp") as temp:
        root = Path(temp)
        home, binary, workspace = root / "home", root / "bin", root / "workspace"
        for p in (home, binary, workspace):
            p.mkdir()
        socket = str(root / "tmux.sock")
        env = {**os.environ, "HOME": str(home), "USERPROFILE": str(home),
               "PATH": str(binary) + os.pathsep + os.environ["PATH"],
               "HEV_ORCHESTRATOR": "", "GIT_CONFIG_NOSYSTEM": "1",
               "HEV_PEERS_BACKEND": "", "HEV_PEERS_STATE": str(root / "peers"),
               "HEV_CONFIG": str(root / "config.toml"), "PEERS_SELF": "test-host"}
        env.pop("TMUX", None)
        env.pop("TMUX_PANE", None)
        config = home / ".config/hev/dispatch.toml"
        config.parent.mkdir(parents=True)
        config.write_text(f"cap = 3\nworkspace = '{workspace}'\ndefault_host = 'local'\n[hosts.local]\nlocal = true\n")
        env["HEV_DISPATCH_CONFIG"] = str(config)
        (binary / "tmux").write_text(f"#!/bin/sh\nexec {shlex.quote(real_tmux)} -S {shlex.quote(socket)} \"$@\"\n")
        (binary / "codex").write_text("""#!/bin/sh
if [ -f packet.json ]; then
    printf 'Goal pending. Next: await operator.\\n' > checkpoint.md
    printf 'Decision needed: approve next step.\\n' > reply.md
else
    printf 'Fake harness completed.\\n' > ../report.md
fi
""")
        shutil.copyfile(fixture, binary / "hev-fake-source")
        for p in binary.iterdir():
            p.chmod(0o700)
        def run(*args):
            result = subprocess.run(args, env=env, text=True, capture_output=True)
            assert result.returncode == 0, (args, result.stdout, result.stderr)
            return result.stdout
        def wait_for(check):
            deadline = time.monotonic() + 10
            while not check():
                if time.monotonic() >= deadline:
                    evidence = {}
                    for path in (home / "dispatch").rglob("*"):
                        if path.name in {"state.json", "result.json", "supervisor.log", "trace"}:
                            evidence[str(path.relative_to(home))] = path.read_text(errors="replace")
                    raise AssertionError(f"acceptance boundary timed out: {evidence}")
                time.sleep(0.02)
        repo = workspace / "app"
        run("git", "init", "-b", "main", str(repo))
        run("git", "-C", str(repo), "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
        origin = root / "origin.git"
        run("git", "init", "--bare", "-b", "main", str(origin))
        run("git", "-C", str(repo), "remote", "add", "origin", str(origin))
        run("git", "-C", str(repo), "push", "-u", "origin", "main")
        template = root / "fix.md"
        template.write_text("---\nname: {{.RunName}}\nrepo: app\nissue: {{.Event.ID}}\nvalidation: none\n---\nFix {{.Event.URL}}\n\n## Acceptance\n- Complete fake work.\n")
        spec = root / "source.md"
        spec.write_text("""---
kind: orchestrator
name: intake
goal: Process test events
mode: loop
every: 1h
max_lifetime: 2h
agent: off
manages:
  runs: {selector: 'issue=TASK-*'}
  repos: [app]
authority: [dispatch]
budget: {runs_per_day: 1, concurrent_runs: 1}
source:
  plugin: fake-source
  rules:
    - {status: Ready, action: fix, template: fix.md, mode: live}
---
Test intake only.
""")
        state_dir = home / "dispatch/orch-intake"
        def state():
            try:
                return json.loads((state_dir / "state.json").read_text())
            except (FileNotFoundError, json.JSONDecodeError):
                return {}
        try:
            run(hev, "orchestrate", "apply", "-f", str(spec))
            wait_for(lambda: state().get("state") == "waiting")
            listed = json.loads(run(hev, "orchestrate", "ls", "--json"))
            assert len(listed) == 1 and listed[0]["name"] == "intake" and listed[0]["runs_today"] == 1, listed
            assert json.loads((state_dir / "source-cursor.json").read_text())["cursor"] == "batch-1"
            inv = json.loads(run(hev, "dispatch", "ls", "--json"))
            rows = inv["local"]["rows"]
            workers = [r for r in rows if r["kind"] == "run"]
            assert len(workers) == 1 and workers[0]["orchestrator"] == "intake", rows
            assert any(r["kind"] == "orch" and r["name"] == "o-intake" for r in rows), rows
            wait_for(lambda: (home / "dispatch" / workers[0]["name"] / "result.json").exists())
            run(hev, "dispatch", "say", "o-intake", "Poll again")
            wait_for(lambda: state().get("rounds", 0) >= 2 and state().get("state") == "waiting")
            assert len(list((home / "dispatch").glob("src-*/order.md"))) == 1, "replay launched a duplicate"
            # Render preview uses the live render path and writes no receipt.
            markers = sorted(p.name for p in (state_dir / "source-markers").iterdir())
            event_file = root / "event.json"
            event_file.write_text(json.dumps({"id": "TASK-2", "kind": "issue", "url": "https://example.com/2", "fields": {"status": "Ready"}}))
            preview = run(hev, "orchestrate", "render", "intake", "--event", str(event_file))
            assert "issue: TASK-2" in preview and "Fix https://example.com/2" in preview, preview
            assert sorted(p.name for p in (state_dir / "source-markers").iterdir()) == markers, "render wrote a receipt"
            run(hev, "orchestrate", "stop", "intake")
            wait_for(lambda: state().get("state") == "stopped")
            def placeholder_gone():
                sessions = subprocess.run([real_tmux, "-S", socket, "list-sessions", "-F", "#S"], env=env, text=True, capture_output=True)
                return "o-intake" not in sessions.stdout
            wait_for(placeholder_gone)
            # A model-enabled source round uses the same fake harness and session path.
            agent_spec = root / "agent.md"
            run("tmux", "new-session", "-d", "-s", "owner", "sleep 120")
            run(hev, "peers", "inbox", "on", "owner")
            agent_spec.write_text(spec.read_text().replace("name: intake", "name: observer").replace("agent: off", "agent: on").replace("mode: loop", "mode: once").replace("authority: [dispatch]", "reply: owner\nauthority: [dispatch, notify]"))
            run(hev, "orchestrate", "apply", "-f", str(agent_spec))
            agent_dir = home / "dispatch/orch-observer"
            wait_for(lambda: (agent_dir / "checkpoint.md").exists())
            def observer_complete():
                try:
                    return json.loads((agent_dir / "state.json").read_text()).get("state") == "complete"
                except (FileNotFoundError, json.JSONDecodeError):
                    return False
            wait_for(observer_complete)
            wait_for(lambda: subprocess.run([real_tmux, "-S", socket, "has-session", "-t", "=o-observer"], env=env, capture_output=True).returncode != 0)
            packet = json.loads((agent_dir / "rounds/001/packet.json").read_text())
            assert packet["source"]["events"][0]["id"] == "TASK-1", packet
            replies = list((agent_dir / "notices/reply").glob("*.json"))
            assert len(replies) == 1, replies
            notice = json.loads(replies[0].read_text())
            assert notice["status"] == "delivered", notice
            assert not list((agent_dir / "rounds").glob("*/notice*")), "notices must live outside rounds/"
            shown = run(hev, "orchestrate", "show", "observer")
            assert "last-notice.json" in shown and '"status": "delivered"' in shown, shown
            listed = json.loads(run(hev, "orchestrate", "ls", "--json"))
            observer = next(row for row in listed if row["name"] == "observer")
            assert observer["last_notice"]["status"] == "delivered", observer
            run(hev, "orchestrate", "stop", "observer")
            last = json.loads((agent_dir / "last-notice.json").read_text())
            assert last["status"] == "delivered" and "stopped" in last["message"], last
            inbox = run(hev, "peers", "inbox", "--session", "owner", "--all", "--json")
            assert inbox.count("Decision needed: approve next step.") == 1, inbox
            assert inbox.count("stopped") == 1, inbox
            print("PASS: durable owner notices, reply delivered once, stop notice, show/ls notice status; apply, fake source -> scoped dispatch, fake harness, cursor, JSON inventories, early say, replay idempotency, render preview, stop removes placeholder, agent:on packet")
        finally:
            subprocess.run([real_tmux, "-S", socket, "kill-server"], env=env, capture_output=True)


if __name__ == "__main__":
    main()
