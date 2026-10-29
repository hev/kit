#!/usr/bin/env python3
"""FOH only: read-only, ID-paginated export for the offline schema planner."""
import argparse
import json
import os
from pathlib import Path
from urllib.request import Request, urlopen
from plan import NAMES


def snapshot(output):
    endpoint = os.environ["LAYER_ENDPOINT"].rstrip("/")
    token = os.environ["LAYER_API_KEY"]
    output.mkdir(parents=True, exist_ok=False)

    def read(path, body=None):
        req = Request(endpoint + path, data=None if body is None else json.dumps(body).encode(), headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        with urlopen(req, timeout=180) as response:
            result = json.load(response)
        if isinstance(result, dict) and result.get("error"):
            raise ValueError(result["error"])
        return result

    for namespace in NAMES.values():
        schema = read(f"/v1/namespaces/{namespace}/schema")
        (output / f"{namespace}.schema.json").write_text(json.dumps(schema, indent=2, sort_keys=True) + "\n")
        cursor = None
        with (output / f"{namespace}.rows.jsonl").open("w") as stream:
            while True:
                body = {"rank_by": ["id", "asc"], "top_k": 1000, "include_attributes": True}
                if cursor is not None:
                    body["filters"] = ["id", "Gt", cursor]
                rows = read(f"/v2/namespaces/{namespace}/query", body)["rows"]
                for row in rows:
                    # Embedding values are never needed by the patch/rollback.
                    row.pop("vector", None)
                    stream.write(json.dumps(row, sort_keys=True) + "\n")
                if len(rows) < 1000:
                    break
                next_cursor = rows[-1]["id"]
                if cursor is not None and next_cursor <= cursor:
                    raise ValueError("export pagination did not advance")
                cursor = next_cursor
        print(namespace, "exported")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    snapshot(parser.parse_args().output)
