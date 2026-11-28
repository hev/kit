#!/usr/bin/env python3
"""Isolated Unix FIFO acceptance: real CLI/git/tmux, gated fake harness."""
import concurrent.futures
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import uuid

hev = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="kq-", dir="/tmp") as tmp:
    root = Path(tmp)
    env = dict(os.environ, HOME=tmp, USERPROFILE=tmp)
    env.pop("HEV_ORCHESTRATOR", None)
    env.pop("TMUX", None)
    bin_dir = root / "bin"
    bin_dir.mkdir()
    socket = str(root / "tmux.sock")
    real_tmux = shutil.which("tmux")
    assert real_tmux
    def script(name, body):
        p = bin_dir / name
        p.write_text("#!/bin/sh\n" + body)
        p.chmod(0o700)
    script("tmux", f'exec "{real_tmux}" -S "{socket}" "$@"\n')
    script("gh", "exit 1\n")
    script("codex", 'name=$(basename "$(dirname "$PWD")")\n'
           'echo "$HEV_WORK_ID" > "$HOME/work-id-$name"\n'
           'echo "$name" >> "$HOME/starts"\n'
           'while [ ! -f "$HOME/release-$name" ]; do sleep 0.05; done\n')
    env["PATH"] = str(bin_dir) + os.pathsep + env["PATH"]
    cfg = root / "dispatch.toml"
    cfg.write_text('cap=1\n[hosts.local]\nlocal=true\n')
    env["HEV_DISPATCH_CONFIG"] = str(cfg)
    source = root / "source"
    source.mkdir()
    def run(*args, cwd=None):
        return subprocess.check_output(args, cwd=cwd, env=env, text=True, stderr=subprocess.STDOUT)
    run("git", "init", "-b", "main", cwd=source)
    run("git", "-c", "user.name=Test", "-c", "user.email=test@example.test",
        "commit", "--allow-empty", "-m", "initial", cwd=source)
    repo = root / "repo"
    run("git", "clone", str(source), str(repo))
    def cli(*args):
        return run(hev, "dispatch", *args)
    def submit(name):
        order = root / (name + ".md")
        order.write_text(f"---\nname: {name}\nrepo: {repo}\nhost: local\n---\nTask\n## Acceptance\n- done\n")
        return cli(str(order))
    def wait_for(check, label):
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            if check():
                return
            time.sleep(0.05)
        raise AssertionError("timed out: " + label)
    def starts():
        p = root / "starts"
        return p.read_text().splitlines() if p.exists() else []
    def sessions():
        # Each supervisor is the only process of its private-tmux session
        # d-<name>; the session (and the server, once empty) ends when it exits.
        r = subprocess.run([real_tmux, "-S", socket, "list-sessions", "-F", "#{session_name}"], capture_output=True, text=True)
        return r.stdout.split() if r.returncode == 0 else []
    def inventory():
        return json.loads(cli("ls", "--json"))["local"]["rows"]
    try:
        assert "started first" in submit("first")
        wait_for(lambda: starts() == ["first"], "first start")
        assert "position 1" in submit("second")
        assert "position 2" in submit("third")
        rows = inventory()
        assert [(r["name"], r["position"]) for r in rows if r["state"] == "queued"] == [("second", 1), ("third", 2)], rows
        ids = {r["name"]: r["work_id"] for r in rows}
        assert len(set(ids.values())) == len(ids)
        for name, work_id in ids.items():
            assert str(uuid.UUID(work_id)) == work_id != str(uuid.UUID(int=0))
            assert f'"work_id": "{work_id}"' in cli("show", name)
        print(cli("ls"), end="")
        (root / "release-first").touch()
        wait_for(lambda: starts() == ["first", "second"], "completion starts second")
        (root / "release-second").touch()
        wait_for(lambda: starts() == ["first", "second", "third"], "completion starts third")
        # Kill the actual supervisor while its harness is still gated. A
        # backstop invocation must recover capacity and stop the orphan.
        assert "position 1" in submit("after-crash")
        supervisor = json.loads((root / ".hev/orders/third/supervisor.json").read_text())["pid"]
        os.kill(supervisor, signal.SIGKILL)
        wait_for(lambda: subprocess.run([real_tmux, "-S", socket, "has-session", "-t", "d-third"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0, "dead supervisor session")
        cli("sweep")
        wait_for(lambda: starts()[-1:] == ["after-crash"], "backstop starts after crash")
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(submit, ["cancel-one", "cancel-two"]))
        assert sorted("position 1" in r for r in results) == [False, True], results
        assert all("queued" in r for r in results), results
        assert "cancelled queued" in cli("stop", "cancel-one")
        assert "cancelled queued" in cli("stop", "cancel-two")
        assert not [r for r in inventory() if r["state"] == "queued"]
        (root / "release-after-crash").touch()
        wait_for(lambda: (root / ".hev/orders/after-crash/result.json").exists() and json.loads((root / ".hev/orders/after-crash/result.json").read_text()).get("end", "0001").startswith("20"), "last result")
        assert starts() == ["first", "second", "third", "after-crash"], starts()
        # result.json's end is not the supervisor's exit: it still writes the
        # attention notice, removes its records and drains the queue in HOME.
        # Removing HOME under it fails rmtree, so wait for every supervisor.
        wait_for(lambda: not sessions(), "supervisors exit after the last result")
        for name in ("first", "second", "after-crash"):
            worker_id = (root / f"work-id-{name}").read_text().strip()
            meta = json.loads((root / f".hev/orders/{name}/meta.json").read_text())
            result = json.loads((root / f".hev/orders/{name}/result.json").read_text())
            assert worker_id == meta["work_id"] == result["work_id"]
            if name in ids:
                assert worker_id == ids[name]
        assert (root / "work-id-third").read_text().strip() == ids["third"]
        print("PASS: persistent work UUIDs in show, list, workers and results")
        print("PASS: FIFO completion, positions/list, killed-supervisor sweep, concurrent enqueue, cancel")
    finally:
        for name in ["first", "second", "third", "after-crash"]:
            (root / ("release-" + name)).touch()
        # After a failure, let released supervisors finish before the backstop.
        deadline = time.monotonic() + 30
        while sessions() and time.monotonic() < deadline:
            time.sleep(0.05)
        subprocess.run([real_tmux, "-S", socket, "kill-server"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        # Remove the temporary worktrees/branches through their owner repo.
        subprocess.run(["git", "-C", str(repo), "worktree", "prune"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
