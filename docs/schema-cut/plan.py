#!/usr/bin/env python3
"""Offline only: exported schemas + JSONL rows -> reviewable patches and rollback."""
import argparse
import copy
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
NAMES = {"chunks": "hev-traces", **{k: "hev-traces-" + k for k in ("sessions", "blocks", "evals", "instructions")}}


def dynamic(key):
    return key.startswith(("commit_outcome_", "ci_workflow_", "mark_"))


def fold(row, kind):
    target = {"sessions": "outcome_details", "evals": "marks"}.get(kind)
    if not target:
        return None, None
    legacy = {k: v for k, v in row.items() if dynamic(k) and v is not None}
    if not legacy:
        return None, None
    original = row.get(target)
    compact = json.loads(original) if isinstance(original, str) else original or {}
    if not isinstance(compact, dict):
        raise ValueError(f'{row["id"]}: {target} is not an object')
    compact = dict(compact)
    for key, value in legacy.items():
        name = key[5:] if kind == "evals" else key
        if name in compact and compact[name] != value:
            raise ValueError(f'{row["id"]}: conflicting compact/legacy value {key}; resolve snapshot explicitly')
        compact[name] = value
    patch = {"id": row["id"], target: json.dumps(compact, sort_keys=True, separators=(",", ":")), **dict.fromkeys(legacy)}
    rollback = {"id": row["id"], target: original, **legacy}
    return patch, rollback


def schema_plan(schema, indexed, kind):
    namespace = NAMES[kind]
    result = json.loads((ROOT / f"{namespace}.schema.json").read_text())
    for key, definition in schema.items():
        if key in ("id", "vector"):
            continue
        definition = copy.deepcopy(definition)
        # Existing embedding configuration is never redeclared during repair.
        definition.pop("embed", None)
        definition["filterable"] = key in indexed
        result[key] = definition
    target = {"sessions": "outcome_details", "evals": "marks"}.get(kind)
    if target:
        result.setdefault(target, {"type": "string", "filterable": False})
    return result


def indexed_count(schema):
    # Matches turbopuffer's implicit scalar-index default, excluding built-ins.
    return sum(k not in ("id", "vector") and d.get("filterable", not any(d.get(x) for x in ("full_text_search", "regex", "glob", "fuzzy"))) is True for k, d in schema.items())


def plan(source, output):
    policy = json.loads((ROOT / "policy.json").read_text())
    output.mkdir(parents=True, exist_ok=False)
    report = {}
    for kind, namespace in NAMES.items():
        schema = json.loads((source / f"{namespace}.schema.json").read_text())
        repaired = schema_plan(schema, policy[kind], kind)
        (output / f"{namespace}.schema.json").write_text(json.dumps(repaired, indent=2, sort_keys=True) + "\n")
        rollback_schema = {k: {x: v for x, v in d.items() if x != "embed"} for k, d in schema.items() if k not in ("id", "vector")}
        # Newly declared fields have no original index to restore.
        for key, definition in repaired.items():
            if key not in rollback_schema:
                rollback_schema[key] = {**definition, "filterable": False}
        # Explicitly restore implicit original filterability as well.
        for k, d in rollback_schema.items():
            if k in schema:
                d["filterable"] = bool(indexed_count({k: schema[k]}))
        (output / f"{namespace}.rollback-schema.json").write_text(json.dumps(rollback_schema, indent=2, sort_keys=True) + "\n")
        rows = changed = nulled = 0
        seen = set()
        with (source / f"{namespace}.rows.jsonl").open() as inp, (output / f"{namespace}.patches.jsonl").open("w") as patches, (output / f"{namespace}.rollback.jsonl").open("w") as rollback:
            for line in inp:
                row = json.loads(line)
                if row["id"] in seen:
                    raise ValueError(f"duplicate physical row ID {namespace}/{row['id']}")
                seen.add(row["id"])
                rows += 1
                patch, undo = fold(row, kind)
                if patch:
                    # One bounded request per line; patching never re-embeds text.
                    patches.write(json.dumps({"patch_rows": [patch]}, sort_keys=True) + "\n")
                    rollback.write(json.dumps({"patch_rows": [undo]}, sort_keys=True) + "\n")
                    changed += 1
                    nulled += sum(v is None for k, v in patch.items() if dynamic(k))
        report[namespace] = {"rows": rows, "rewrite_rows": changed, "nulled_values": nulled, "attributes_before": len(schema), "indexed_before": indexed_count(schema), "indexed_after": indexed_count(repaired), "dynamic_columns": sum(dynamic(k) for k in schema)}
    (output / "report.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps(report, indent=2, sort_keys=True))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshot", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    plan(args.snapshot, args.output)
