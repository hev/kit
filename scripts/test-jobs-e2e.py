#!/usr/bin/env python3
"""Real minute slots and one daemon restart, entirely under a throwaway HOME.
Usage: python3 scripts/test-jobs-e2e.py /absolute/path/to/hev [evidence-directory]
Never installs kit or contacts a secret provider.
"""
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time

binary = str(Path(sys.argv[1]).resolve())
evidence = Path(sys.argv[2]).resolve() if len(sys.argv) > 2 else Path(tempfile.mkdtemp(prefix="jobs-evidence-"))
evidence.mkdir(parents=True, exist_ok=True)
home = Path(tempfile.mkdtemp(prefix="hev-jobs-"))
# Keep Unix-domain socket paths below the platform limit.
transient_home = home
env = {"HOME": str(home), "USERPROFILE": str(home), "PATH": "/usr/local/bin:/usr/bin:/bin", "LANG": "C.UTF-8"}
transcript = (evidence / "transcript.txt").open("w")
output = home / "occurrences.txt"
job = home / "minute.md"
job.write_text(f"""---
kind: job
name: minute-proof
host: {socket.gethostname()}
exec:
  argv: [/bin/sh, -c, 'echo "$HEV_JOB_OCCURRENCE" >> occurrences.txt; echo "slot $HEV_JOB_OCCURRENCE"']
  cwd: {home}
schedule:
  every: 1m
  catch_up: none
timeout: 10s
reply: owner-fixture
---
Throwaway acceptance fixture; no secrets or external effects.
""")

def run(*args):
    transcript.write("$ hev " + " ".join(args) + "\n")
    p = subprocess.run([binary, *args], env=env, text=True, capture_output=True)
    transcript.write(p.stdout + p.stderr + f"exit={p.returncode}\n")
    transcript.flush()
    if p.returncode:
        raise RuntimeError(f"command failed: {args}")
    return json.loads(p.stdout)

log = (evidence / "daemon.log").open("a")
daemon = None

def start():
    p = subprocess.Popen([binary, "daemon", "--secrets-only"], env=env, stdout=log, stderr=log)
    transcript.write(f"daemon started pid={p.pid}\n")
    transcript.flush()
    ready = time.monotonic() + 10
    while not (home / ".hev/jobs/scheduler.lock").exists():
        if p.poll() is not None or time.monotonic() > ready:
            raise RuntimeError("daemon did not start its isolated scheduler")
        time.sleep(0.05)
    return p

def stop(p):
    p.terminate()
    code = p.wait(timeout=30)
    transcript.write(f"daemon stopped pid={p.pid} exit={code}\n")
    transcript.flush()
    if code:
        raise RuntimeError(f"daemon exit {code}")

try:
    daemon = start()
    run("jobs", "apply", "-f", str(job), "--dry-run", "--json")
    run("jobs", "apply", "-f", str(job), "--json")
    deadline = time.monotonic() + 230
    restarted = False
    while time.monotonic() < deadline:
        if daemon.poll() is not None:
            raise RuntimeError("throwaway daemon exited early")
        lines = output.read_text().splitlines() if output.exists() else []
        if len(lines) >= 1 and not restarted:
            stop(daemon)
            daemon = None
            time.sleep(2)
            daemon = start()
            restarted = True
        if len(lines) >= 3:
            break
        time.sleep(1)
    else:
        raise RuntimeError("fewer than three minute occurrences before deadline")
    # Let the scheduler observe the final command's terminal dispatch record.
    time.sleep(2)
    run("jobs", "ls", "--json")
    state = run("jobs", "show", "minute-proof", "--json")
    history = run("jobs", "history", "minute-proof", "--json")
    run("jobs", "logs", "minute-proof", "--json")
    assert restarted and len(lines) == 3 and len(set(lines)) == 3, lines
    assert len(history) == 3 and all(o["state"] == "succeeded" for o in history), history
    assert all(o["id"] == lines[i] for i, o in enumerate(history)), history
    transcript.write("PASS: three unique minute slots across one daemon restart; all three succeeded\n")
    print(f"PASS: three unique minute slots across one daemon restart; transcript {evidence / 'transcript.txt'}")
finally:
    if daemon is not None and daemon.poll() is None:
        stop(daemon)
    transcript.close()
    log.close()
