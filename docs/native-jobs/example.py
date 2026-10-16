#!/usr/bin/env python3
"""Executable isolated Linux source preflight and real effect-count-one proof."""
import datetime
import hashlib
import json
import os
import pathlib
import subprocess
import sys

binary = os.path.abspath(sys.argv[1])
scratch = pathlib.Path(sys.argv[2]).resolve()
scratch.mkdir(mode=0o700, parents=True, exist_ok=False)
journal = scratch / "journal"

def call(action, **fields):
    p = subprocess.run([binary, "native-job", "--root", str(journal), "--host", "synthetic-host", "--executor", "synthetic-executor"], input=json.dumps(dict(version="native-jobs/v1", action=action, **fields)), text=True, capture_output=True, check=True)
    return json.loads(p.stdout)["result"]

identity = dict(order_uid="synthetic-order", scope_digest="scope", plan_digest="plan", authority_digest="authority", execution_epoch="epoch-1", workflow_uid="workflow-1", node_id="node-1", step_id="build", attempt_id="attempt-1", candidate_digest="candidate", input_digest="inputs", environment_digest="environment", operation="exec", host_id="synthetic-host", executor_id="synthetic-executor", chef_id="chef", fence=1)
expires = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat().replace("+00:00", "Z")
call("fence", fence=dict(order_uid=identity["order_uid"], chef_id="chef", epoch=1, expires=expires))
request = dict(version="native-jobs/v1", intent_key="synthetic-one", payload_digest="", identity=identity, argv=["/bin/sh", "-c", "printf 'effect\\n' >> effects; printf 'finished\\n'"], directory=str(scratch), environment=[])
# This specimen uses ASCII values and the declaration order of Request/Identity.
request["payload_digest"] = "sha256:" + hashlib.sha256(json.dumps(request, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest()
j = call("submit", request=request)
assert call("submit", request=request) == j  # lost submit ACK
assert call("query", job_id=request["intent_key"]) == j
j = call("execute", job_id=j["id"])
assert j["state"] == "Completed" and j["result"]["exit_code"] == 0
assert (scratch / "effects").read_text() == "effect\n"
events = call("outbox")
assert len(events) == 1 and call("outbox") == events  # lost result ACK
event = events[0]
for _ in range(2):
    call("ack", job_id=j["id"], identity=identity, event_id=event["event_id"], result_digest=event["result_digest"])
assert call("outbox") == []
assert call("query", job_id=j["id"])["acknowledged"]
assert (scratch / "effects").read_text() == "effect\n"
print(json.dumps(dict(evidence_class="isolated-linux-source", passed=True, effect_count=1, job_id=j["id"], event_id=event["event_id"], mac_native=False, live=False)))
