#!/usr/bin/env python3
"""Kit CLI acceptance hook for the disposable Layer Postgres fixture."""
import json
import os
import subprocess
import uuid


def ref():
    return str(uuid.uuid4())


def main():
    env = os.environ.copy()
    env["HEV_WORK_IDENTITY_ENDPOINT"] = env.pop("WORK_IDENTITY_FIXTURE_LAYER_URL")
    env["HEV_WORK_IDENTITY_KEY"] = env.pop("WORK_IDENTITY_FIXTURE_BEARER")
    namespace = env.pop("WORK_IDENTITY_FIXTURE_NAMESPACE")
    assert namespace.startswith("work-")
    workspace = namespace.removeprefix("work-")
    assert env["HEV_WORK_IDENTITY_ENDPOINT"].startswith("http://127.0.0.1:")
    assert env["HEV_WORK_IDENTITY_KEY"].startswith("hvl_fixture_")
    hev = env["KIT_WORK_IDENTITY_TEST_HEV"]

    def run(*args, error=None, overrides=None):
        result = subprocess.run(
            [hev, "dispatch", "identity", "--workspace", workspace, *map(str, args)],
            env=env | (overrides or {}), capture_output=True, text=True, timeout=10,
            check=False,
        )
        if error is None:
            if result.returncode != 0:
                raise AssertionError(f"{args[0]} failed: {result.stderr}")
            return json.loads(result.stdout)
        assert result.returncode != 0 and not result.stdout, (args[0], result.returncode)
        assert error in result.stderr, (args[0], result.stderr)

    key, request, scope = ref(), ref(), ref()
    root = run("create", key, request, scope)
    root_id, old_root = root["id"], root["address"]
    assert root["workspace_id"] == workspace and root["parent_id"] is None
    assert root["acceptance"] is None and root["request_ref"] == request
    child = run("create", ref(), ref(), ref(), "--parent", root_id)
    child_id, old_child = child["id"], child["address"]
    assert child["parent_id"] == root_id and child_id != root_id

    root = run("move", root_id, root["revision"], root["segments"][0] + 10)
    assert root["address"] != old_root
    assert run("resolve", old_root) == root
    child = run("resolve", old_child)
    assert child["id"] == child_id and child["address"] != old_child
    assert child["address"].startswith(root["address"] + ".")
    assert run("create", key, request, scope) == root
    run("create", key, request, ref(), error="HTTP 409")
    print("PASS: authenticated UUIDs, canonical aliases, atomic subtree move, create replay and conflict")

    child_evidence, root_evidence = ref(), ref()
    child = run("link", child_id, child["revision"], "artifact", child_evidence)
    assert run("link", child_id, child["revision"], "artifact", child_evidence) == child
    child = run("accept", child_id, child["revision"], child["scope_revision"], child_evidence)
    assert child["acceptance"]["evidence_ref"] == child_evidence
    assert run("get", root_id)["acceptance"] is None
    run("accept", root_id, root["revision"], root["scope_revision"], child_evidence,
        error="HTTP 409")
    root = run("link", root_id, root["revision"], "execution", ref())
    root = run("link", root_id, root["revision"], "artifact", root_evidence)
    root = run("accept", root_id, root["revision"], root["scope_revision"], root_evidence)
    assert root["acceptance"]["evidence_ref"] == root_evidence
    revised_scope = ref()
    revised = run("revise", root_id, root["revision"], revised_scope)
    assert revised["scope_ref"] == revised_scope and revised["acceptance"] is None
    assert revised["scope_revision"] == root["scope_revision"] + 1
    assert run("get", child_id)["acceptance"] == child["acceptance"]
    print("PASS: separate root and child evidence and acceptance, scope revision invalidates root only")

    first = run("list", "--limit", 1)
    assert first["has_more"] and [a["id"] for a in first["items"]] == [root_id]
    second = run("list", "--limit", 1, "--after", first["next_after"])
    assert not second["has_more"] and [a["id"] for a in second["items"]] == [child_id]
    empty = run("list", "--after", second["next_after"])
    assert empty == {"items": [], "next_after": None, "has_more": False}

    history = []
    cursor = 0
    for _ in range(20):
        page = run("history", "--limit", 2, "--after", cursor)
        history.extend(page["items"])
        assert page["next_after"] > cursor and page["next_after"] == history[-1]["cursor"]
        cursor = page["next_after"]
        if not page["has_more"]:
            break
    else:
        raise AssertionError("history did not finish within 20 pages")
    assert [item["cursor"] for item in history] == sorted({item["cursor"] for item in history})
    assert any(item["ask"]["id"] == root_id and item["ask"]["acceptance"] == root["acceptance"]
               for item in history)
    assert run("history", "--after", cursor) == empty
    run("get", root_id, error="HTTP 401", overrides={"HEV_WORK_IDENTITY_KEY": "synthetic-invalid"})
    print("PASS: bounded list and history cursor polling, retained acceptance history, authorization refusal")


if __name__ == "__main__":
    main()
