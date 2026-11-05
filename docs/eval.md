# Worker evaluation evidence

`hev eval put [file]` stores evaluation JSON rows. Evaluate observed behavior
against the actual order and record evidence references in the evaluation;
unrun checks are unknown, and failed or flaky tests must be named.

For a user-visible change with a deployed preview required by its order, missing
preview evidence is a poor result unless the order's allowed fallback records a
substantiated substitute. A failed deployment alone does not establish poor
worker behavior: distinguish the deployment failure from how the worker
investigated, reported and substantiated the result. Do not infer an arbitrary
time limit or preview requirement for orders that do not specify one.

See [evaluation storage](rfcs/0005-marks.md) for the JSON contract and
[historical replay](runbooks/eval-corpus-replay.md) for preserving imported rows.
