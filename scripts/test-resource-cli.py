#!/usr/bin/env python3
"""Exercise the real hev/Factory candidate pair without shared effects."""
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile

hev, backend = [str(Path(p).resolve()) for p in sys.argv[1:]]
with tempfile.TemporaryDirectory(prefix="hev-resources-") as scratch:
    root = Path(scratch)
    home = root / "absent"
    bridge = root / "backend"
    bridge.write_text("#!/bin/sh\nexec " + shlex.quote(backend) + ' --local "$@"\n')
    bridge.chmod(0o700)
    env = dict(os.environ, FACTORY_HOME=str(home), HEV_RESOURCE_BACKEND=str(bridge))
    def call(*args, data=None, failure=None):
        p = subprocess.run([hev, *args], input=data, text=True, capture_output=True, env=env, timeout=15)
        if failure:
            assert p.returncode != 0 and not p.stdout, (args, p.stdout, p.stderr)
            assert failure in p.stderr, (args, p.stderr)
            return None
        assert p.returncode == 0 and not p.stderr, (args, p.stdout, p.stderr)
        return p.stdout
    discovery = json.loads(call("api-resources", "-o", "json"))
    resource = discovery["resources"][0]
    assert resource["kind"] == "Order" and not resource["activation"]
    assert set(resource["unsupported"]) == {"logs", "watch", "wait"}
    fixture = {"apiVersion": "factory/v1alpha1", "kind": "Order", "metadata": {"name": "example", "labels": {"purpose": "contract-example"}}, "spec": {"ask": "Build a candidate", "authorityRef": "fixture:no-authority", "acceptance": "Independent exact candidate evidence", "steps": [{"id": "build", "task": "Produce candidate"}]}}
    declaration = json.dumps(fixture)
    preview = json.loads(call("apply", "-f", "-", "--dry-run", "-o", "json", data=declaration))
    assert not preview["status"]["persisted"] and not home.exists()
    assert json.loads(call("get", "orders", "-o", "json"))["items"] == [] and not home.exists()
    call("apply", "-f", "-", "--dry-run=false", data=declaration, failure="InvalidArgument")
    assert not home.exists()
    created = json.loads(call("apply", "-f", "-", "-o", "json", data=declaration))
    assert created["metadata"]["uid"] and created["metadata"]["generation"] == 1
    assert created == json.loads(call("get", "order/example", "-o", "json"))
    filtered = json.loads(call("get", "orders", "-l", "purpose=contract-example,!retired", "--limit", "1", "-o", "json"))
    assert filtered["items"] == [created] and not filtered["truncated"]
    assert "Observed generation: 1" in call("describe", "order/example")
    assert "apiVersion: factory/v1alpha1" in call("get", "orders/example", "-o", "yaml")
    for verb in ("logs", "watch", "wait"):
        call(verb, "order/example", failure="UnsupportedCapability")
    call("wait", "--timeout", "1ms", "order/example", failure="UnsupportedCapability")
    fixture["metadata"] = created["metadata"]
    fixture["spec"]["ask"] = "Revised candidate"
    revision = json.dumps(fixture)
    updated = json.loads(call("apply", "-f", "-", "-o", "json", data=revision))
    assert updated["metadata"]["uid"] == created["metadata"]["uid"] and updated["metadata"]["generation"] == 2
    call("apply", "-f", "-", data=revision, failure="Conflict")
    for command in ("trace", "query", "ls", "index"):
        assert command in call(command, "--help")
print("PASS: real hev/Factory discovery, zero effects, typed output, filters, UID/CAS/generation, unsupported observations and retained trace commands")
