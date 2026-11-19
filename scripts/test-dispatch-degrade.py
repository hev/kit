#!/usr/bin/env python3
"""Isolated Unix degrade acceptance: real CLI, git and (private) tmux.

A fake claude replays the captured Claude Code usage-limit record
(internal/dispatch/testdata/degrade/claude-stream-usage-limit.jsonl, from the
2026-11-19 follow-through incident) for claude-fable-5-2 and succeeds on any
other model. The run must degrade to Opus 5.5, show it in `hev dispatch ls`
and `show`, and fire the on_degrade hook, which appends its JSON to a file.
No real model is launched and no plan usage is spent.

usage: test-dispatch-degrade.py HEV_BINARY [--keep]
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

hev = str(Path(sys.argv[1]).resolve())
keep = "--keep" in sys.argv[2:]
here = Path(__file__).resolve().parent.parent
fixture = here / "internal/dispatch/testdata/degrade/claude-stream-usage-limit.jsonl"

tmp = tempfile.mkdtemp(prefix="kdeg-", dir=os.environ.get("TMPDIR", "/tmp"))
root = Path(tmp)
env = dict(os.environ, HOME=tmp, USERPROFILE=tmp)
for k in ("HEV_ORCHESTRATOR", "TMUX", "TMUX_PANE", "PEERS_FROM"):
    env.pop(k, None)
bin_dir = root / "bin"
bin_dir.mkdir()
socket = str(root / "tmux.sock")
real_tmux = shutil.which("tmux")
assert real_tmux, "tmux is required"


def script(name, body):
    p = bin_dir / name
    p.write_text("#!/bin/sh\n" + body)
    p.chmod(0o700)


# A reset two days out keeps the record's shape and a future reset.
reset = int(time.time()) + 48 * 3600
(root / "claude-limit.jsonl").write_text(fixture.read_text().replace("1795266000", str(reset)))
script("tmux", f'exec "{real_tmux}" -S "{socket}" "$@"\n')
script("gh", "exit 1\n")
script("claude", f"""model=""
prev=""
for a; do
  if [ "$prev" = "--model" ]; then model="$a"; fi
  prev="$a"
done
echo "claude $model" >> "$HOME/launches"
if [ "$model" = "claude-fable-5-2" ]; then
  cat "{root}/claude-limit.jsonl"
  exit 1
fi
printf '{{"type":"system","subtype":"init","session_id":"stub-%s"}}\\n' "$model"
printf 'stub worker on %s\\n' "$model" > ../report.md
printf 'worked\\ndone\\n' > ../status.md
printf '{{"type":"result","subtype":"success","is_error":false,"result":"done"}}\\n'
""")
hook_log = root / "on-degrade.jsonl"
script("on-degrade", f'cat >> "{hook_log}"\n')
env["PATH"] = str(bin_dir) + os.pathsep + env["PATH"]
cfg = root / "dispatch.toml"
cfg.write_text(f'cap = 1\nworkspace = "{root}"\n[hosts.oven]\nlocal = true\n\n'
               f'[degrade]\non_degrade = ["{bin_dir / "on-degrade"}"]\nhook_timeout = "5s"\n')
env["HEV_DISPATCH_CONFIG"] = str(cfg)


def run(*args, cwd=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True, stderr=subprocess.STDOUT)


source = root / "source"
source.mkdir()
run("git", "init", "-q", "-b", "main", cwd=source)
run("git", "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "-q", "--allow-empty", "-m", "initial", cwd=source)
repo = root / "demo"
run("git", "clone", "-q", str(source), str(repo))

name = "live-degrade-check"
order = root / (name + ".md")
order.write_text(f"---\nname: {name}\nhost: oven\nrepo: {repo}\nharness: claude\nmodel: claude-fable-5-2\ntimeout: 5\n---\n"
                 "Throwaway order for the degrade live check. Do nothing.\n\n## Acceptance\n\n- The stub worker writes report.md\n")


def wait_for(check, label, timeout=60):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if check():
            return
        time.sleep(0.1)
    raise AssertionError("timed out: " + label)


def rows():
    return json.loads(run(hev, "dispatch", "ls", "--json"))["oven"]["rows"]


try:
    print("$ hev dispatch", order.name)
    print(run(hev, "dispatch", str(order)), end="")
    wait_for(lambda: any(r["name"] == name and r["state"] not in ("running", "queued") for r in rows()), "run end")
    print("\n$ hev dispatch ls --all")
    ls = run(hev, "dispatch", "ls", "--all")
    print(ls, end="")
    row = next(r for r in rows() if r["name"] == name)
    assert row["state"] == "done", row
    assert row.get("model_running") == "opus-5-5↓", row
    assert "opus-5-5↓" in ls, ls
    print("\n$ hev dispatch show", name, "(models and result)")
    show = run(hev, "dispatch", "show", name)
    models = show[show.index("Models:"):show.index("--- meta.json ---")]
    print(models, end="")
    assert "claude/claude-fable-5-2: order; exit 1; limit usage_limit_reached seven_day_overage_included" in models, models
    assert "claude/claude-opus-5-5: degrade: limit; exit 0" in models, models
    result = json.loads(json.loads(run(hev, "dispatch", "show", name, "--json"))["files"]["result.json"])
    print(json.dumps({k: result[k] for k in ("models", "degrades")}, indent=2, ensure_ascii=False))
    assert (root / "launches").read_text().split("\n")[:2] == ["claude claude-fable-5-2", "claude claude-opus-5-5"]
    wait_for(lambda: hook_log.exists() and hook_log.read_text().strip(), "on_degrade hook")
    print("\n$ cat", hook_log.name, "(written by the on_degrade hook)")
    payload = hook_log.read_text()
    print(payload, end="")
    event = json.loads(payload.splitlines()[0])
    assert event["event"] == "degrade" and event["run"] == name and event["step"] == "worker", event
    assert event["from"]["model"] == "claude-fable-5-2" and event["to"]["model"] == "claude-opus-5-5", event
    assert event["limit_type"] == "seven_day_overage_included" and event["resets_at"], event
    log = (root / "dispatch" / name / "log").read_text()
    assert '"type":"dispatch.degrade"' in log
    print("\nPASS: degraded claude-fable-5-2 → claude-opus-5-5; ls, show and on_degrade verified")
finally:
    subprocess.run([real_tmux, "-S", socket, "kill-server"], env=env, stderr=subprocess.DEVNULL)
    if keep:
        print("kept", tmp)
    else:
        shutil.rmtree(tmp, ignore_errors=True)
