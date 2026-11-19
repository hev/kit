#!/usr/bin/env python3
"""Turn real Antigravity CLI (agy) conversations into reviewable test fixtures.

Usage:
  generate-agy-fixture.py OUT_DIR AGY_ROOT NAME=CONVERSATION_ID [...]

For each NAME=ID it copies AGY_ROOT/conversations/ID.db (and its WAL) to a
scratch file, keeps only what kit's decoder reads, and writes OUT_DIR/NAME.sql
(a sqlite3 .dump). It also writes OUT_DIR/conversation_summaries.sql holding
those conversations' rows of the conversation_summaries table (agy's search
index tables in the same database are left out). Run it on a host where the conversations
were made from harmless prompts in a scratch directory.

What is dropped, so the fixtures carry no system prompt, no per-user
configuration and no opaque signatures:
  - steps.metadata (a copy of the step's own metadata) and the other side
    columns;
  - from step payloads: user_config / last_user_config of user input, thought
    and thinking signatures, a tool step's typed payload Any, and a terminal
    block's rendered output copy;
  - from gen_metadata: everything but step_indices and the chat model's
    model, usage, response_model and model_display_name;
  - executor_metadata rows; trajectory_metadata_blob fields other than
    workspaces, created_at, parent/root conversation and workspace_uris.
The user's home directory is rewritten to /home/agy (same length, so no
protobuf length prefix moves).
"""
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile


def varint(b, i):
    r = s = 0
    while True:
        c = b[i]
        i += 1
        r |= (c & 0x7F) << s
        s += 7
        if c < 0x80:
            return r, i


def enc_varint(v):
    out = bytearray()
    while True:
        c = v & 0x7F
        v >>= 7
        if v:
            out.append(c | 0x80)
        else:
            out.append(c)
            return bytes(out)


def fields(b):
    i = 0
    while i < len(b):
        k, j = varint(b, i)
        num, wire = k >> 3, k & 7
        if wire == 0:
            _, e = varint(b, j)
            yield num, wire, b[i:e], None
        elif wire == 1:
            yield num, wire, b[i:j + 8], None
            e = j + 8
        elif wire == 5:
            yield num, wire, b[i:j + 4], None
            e = j + 4
        elif wire == 2:
            n, s = varint(b, j)
            e = s + n
            yield num, wire, b[i:s], b[s:e]
        else:
            raise ValueError("unsupported wire type %d" % wire)
        i = e


def rewrite(b, rule):
    """rule maps field number -> None (drop), True (keep as is) or a nested
    rule (dict) applied to that submessage. "*" is the default for numbers the
    rule does not name (True when absent)."""
    out = bytearray()
    for num, wire, head, payload in fields(b):
        r = rule.get(num, rule.get("*", True))
        if r is None:
            continue
        if isinstance(r, dict) and wire == 2:
            sub = rewrite(payload, r)
            key = enc_varint((num << 3) | 2)
            out += key + enc_varint(len(sub)) + sub
            continue
        out += head + (payload or b"")
    return bytes(out)


TOOL_CALL = {6: None, 7: None}  # ChatToolCall: thought_signature, thinking_signature
STEP = {
    5: {4: TOOL_CALL, 29: TOOL_CALL},  # metadata: tool_call, modified_tool_call
    19: {12: None, 13: None},  # user_input: user_config, last_user_config
    20: {14: None, 7: TOOL_CALL},  # planner_response: thinking_signature, tool_calls
    140: {2: {6: None, 3: {10: {3: {4: None}}}}},  # generic.result: payload, render terminal output
}
GEN = {"*": None, 2: True, 1: {"*": None, 3: True, 4: True, 19: True, 21: True}}
TRAJECTORY = {"*": None, 1: True, 2: True, 5: True, 6: True, 7: True}


def open_ro(path):
    """Open an agy database without writing beside it. With no -wal file agy
    is closed, and a plain read-only open would still create -wal and -shm
    files next to agy's database, so the file is opened immutable."""
    q = "mode=ro" if os.path.exists(path + "-wal") else "mode=ro&immutable=1"
    return sqlite3.connect("file:%s?%s" % (path, q), uri=True)


def scrub(b, home):
    if b is None:
        return None
    new = b"/home/agy"
    if len(home) == len(new):
        b = b.replace(home, new)
    return b


def main():
    if len(sys.argv) < 4:
        sys.exit(__doc__)
    out_dir, root = sys.argv[1], sys.argv[2]
    pairs = [a.split("=", 1) for a in sys.argv[3:]]
    home = os.path.expanduser("~").encode()
    os.makedirs(out_dir, exist_ok=True)
    tmp = tempfile.mkdtemp(prefix="agy-fixture-")
    try:
        for name, cid in pairs:
            src = os.path.join(root, "conversations", cid + ".db")
            dst = os.path.join(tmp, name + ".db")
            # Read through SQLite (never a byte copy of a live file).
            live = open_ro(src)
            copy = sqlite3.connect(dst)
            live.backup(copy)
            live.close()
            copy.execute("UPDATE steps SET metadata = NULL, error_details = NULL, permissions = NULL, task_details = NULL, render_info = NULL")
            for idx, payload in copy.execute("SELECT idx, step_payload FROM steps").fetchall():
                copy.execute("UPDATE steps SET step_payload = ? WHERE idx = ?", (scrub(rewrite(payload, STEP), home), idx))
            for idx, data in copy.execute("SELECT idx, data FROM gen_metadata").fetchall():
                copy.execute("UPDATE gen_metadata SET data = ?, size = 0 WHERE idx = ?", (scrub(rewrite(data, GEN), home), idx))
            for tid, data in copy.execute("SELECT id, data FROM trajectory_metadata_blob").fetchall():
                copy.execute("UPDATE trajectory_metadata_blob SET data = ? WHERE id = ?", (scrub(rewrite(data, TRAJECTORY), home), tid))
            copy.execute("DELETE FROM executor_metadata")
            copy.commit()
            copy.execute("VACUUM")
            copy.close()
            dump = subprocess.run(["sqlite3", dst, ".dump"], check=True, capture_output=True, text=True).stdout
            if home.decode() in dump:
                sys.exit("%s: home directory survived scrubbing" % name)
            with open(os.path.join(out_dir, name + ".sql"), "w") as f:
                f.write(dump)
        ids = [cid for _, cid in pairs]
        sdb = os.path.join(tmp, "summaries.db")
        live = open_ro(os.path.join(root, "conversation_summaries.db"))
        copy = sqlite3.connect(sdb)
        live.backup(copy)
        live.close()
        copy.execute("DELETE FROM conversation_summaries WHERE conversation_id NOT IN (%s)" % ",".join("?" * len(ids)), ids)
        copy.execute("UPDATE conversation_summaries SET raw_summary = NULL, app_data_dir = ''")
        copy.commit()
        copy.execute("VACUUM")
        copy.close()
        dump = subprocess.run(["sqlite3", sdb, ".dump conversation_summaries"], check=True, capture_output=True, text=True).stdout
        if home.decode() in dump:
            sys.exit("conversation_summaries: home directory survived scrubbing")
        with open(os.path.join(out_dir, "conversation_summaries.sql"), "w") as f:
            f.write(dump)
    finally:
        shutil.rmtree(tmp)


if __name__ == "__main__":
    main()
