#!/usr/bin/env python3
"""Exercise workspace boundaries through the CLI with synthetic host transport."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

hev = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="kit-workspaces-") as directory:
    home = Path(directory)
    client = home / "workspace" / "clients" / "bristlecone"
    client.mkdir(parents=True)
    outside = home / "outside"
    outside.mkdir()
    config = home / "config.toml"
    config.write_text("""
[workspaces.hev]
dir = "~/workspace"
hosts = ["tower", "oven", "mini"]

[workspaces.bcc]
dir = "~/workspace/clients/bristlecone"
hosts = ["win-oven"]
login = "own"
""")
    dispatch = home / "dispatch.toml"
    hosts = {"hev": {"tower", "oven", "mini"}, "bcc": {"win-oven"}}
    dispatch.write_text("".join(
        f'[hosts.{host}]\nssh="{host}"\n' for labels in hosts.values() for host in sorted(labels)
    ))
    bin_dir = home / "bin"
    bin_dir.mkdir()
    ssh = bin_dir / "ssh"
    ssh.write_text(r"""#!/usr/bin/env python3
import json, os, sys
host, verb = sys.argv[-2:]
request = json.load(sys.stdin)
workspace = "bcc" if host == "win-oven" else "hev"
with open(os.path.join(os.environ["HOME"], "transport"), "a") as journal:
    journal.write(f"{host} {request['action']}\n")
assert request.get("workspace_name") == workspace
if workspace == "bcc":
    assert "--workspace=bcc" in verb
if request["action"] == "list":
    print(json.dumps({"rows": [
        {"kind": "run", "name": "run-" + host, "workspace": workspace},
        {"kind": "orch", "name": "o-orch-" + host, "workspace": workspace}
    ]}))
elif request["action"] == "start":
    assert request["meta"]["workspace"] == workspace
    print(json.dumps({"message": "started " + request["name"]}))
else:
    print("{}")
""")
    ssh.chmod(0o700)
    state = home / "peers"
    snapshots = state / "hosts"
    snapshots.mkdir(parents=True)
    for host in hosts["hev"] | hosts["bcc"]:
        (snapshots / f"{host}.json").write_text(json.dumps(
            {"host": host, "panes": [], "at": time.time()}
        ))
    env = dict(os.environ, HOME=directory, USERPROFILE=directory,
               HEV_CONFIG=str(config), HEV_DISPATCH_CONFIG=str(dispatch),
               HEV_PEERS_STATE=str(state), PEERS_SELF="reception",
               HEV_ARCHIVE_DIR=str(home / "legacy-archive"),
               HEV_BOARD_DIR=str(home / "legacy-board"),
               PATH=str(bin_dir) + os.pathsep + os.environ["PATH"])
    for key in list(env):
        if key.startswith(("HEVLAYER_", "LAYER_", "FACTORY_BOARD_", "PEERS_HOST_LABEL")) or key in (
            "GITHUB_ACTIONS", "TMUX", "TMUX_PANE", "HEV_WORKSPACE", "HEV_ORCHESTRATOR",
        ):
            env.pop(key)

    def run(*args, cwd=outside, refusal=None, extra=None):
        result = subprocess.run([hev, *args], cwd=cwd, env=env | (extra or {}),
                                text=True, capture_output=True, timeout=25)
        if refusal is not None:
            assert result.returncode and refusal in result.stderr, result
        else:
            assert result.returncode == 0, result
        return result.stdout

    assert json.loads(run("workspace", "current", "--json", cwd=client))["name"] == "bcc"
    assert json.loads(run("-w", "hev", "workspace", "current", "--json", cwd=client))["source"] == "flag"
    assert json.loads(run("workspace", "current", "--json", cwd=client,
                          extra={"HEV_WORKSPACE": "hev"}))["source"] == "env"
    for args, extra in [(("-w", "missing", "workspace", "current"), {}),
                        (("workspace", "current"), {"HEV_WORKSPACE": "missing"}),
                        (("workspace", "use", "missing"), {})]:
        run(*args, extra=extra, refusal="unknown workspace")
    run("workspace", "use", "bcc")
    assert json.loads(run("workspace", "current", "--json"))["source"] == "active"

    for workspace, allowed in hosts.items():
        argv = ["-w", workspace]
        target, denied = ("win-oven", "oven") if workspace == "bcc" else ("oven", "win-oven")
        for action in ("ls", "hosts"):
            assert set(json.loads(run(*argv, "dispatch", action, "--json"))) == allowed
        assert {row["host"] for row in json.loads(run(*argv, "orchestrate", "ls", "--json"))} == allowed
        peers = json.loads(run(*argv, "peers", "ls", "--json"))
        assert {row["host"] for row in peers["hosts"]} == allowed
        assert not peers["panes"]
        journal = home / "transport"
        before = journal.read_text()
        run(*argv, "dispatch", "run", "repo", "task", "--host", denied, refusal="workspace boundary")
        run(*argv, "dispatch", "rework", f"{denied}/run", "retry", refusal="workspace boundary")
        run(*argv, "dispatch", "validate", "run", "--host", denied, refusal="workspace boundary")
        assert journal.read_text() == before
        run(*argv, "dispatch", "run", "repo", "task", "--host", target)
        assert f"{target} start" in journal.read_text()[len(before):]

        run(*argv, "board", "post", "--title", workspace + "-only", "-m", "fixture")
    for workspace, other in (("hev", "bcc"), ("bcc", "hev")):
        board = run("board", "--workspace=" + workspace, "ls")
        assert workspace + "-only" in board and other + "-only" not in board
    run("-w", "bcc", "traces", "list", "--namespace", "hev-traces", refusal="workspace boundary")
    run("-w", "hev", "traces", "list", "--namespace", "bcc-traces", refusal="workspace boundary")
    run("-w", "bcc", "traces", "list")
    run("-w", "hev", "traces", "list")

print("PASS: workspace resolution, both-sided dispatch and inventory, board data, and trace namespace boundaries")
