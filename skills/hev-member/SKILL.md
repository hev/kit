---
name: hev-member
description: Send work to a shared dispatch host you joined as a teammate (hosts named hev-node-*). Use when starting, checking, messaging or stopping your own runs on a host someone else owns.
metadata:
  audience: member
---

A joined host (`hev-node-*` in `hev nodes ls` and `hev dispatch hosts`) runs agents as its owner's account. Your key can start runs and list, show, logs, say and stop only runs bearing your sender name. Pass the host explicitly; never guess routing.

```sh
hev nodes ls
hev dispatch hosts
hev dispatch run example-repo "Fix the failing parser test" --host hev-node-EXAMPLE --accept "go test ./... passes"
hev dispatch order.md --host hev-node-EXAMPLE
hev dispatch ls --host hev-node-EXAMPLE
hev dispatch show example-order --host hev-node-EXAMPLE
hev dispatch logs example-order --host hev-node-EXAMPLE
hev dispatch say example-order "Please also cover the empty input" --host hev-node-EXAMPLE
hev dispatch stop example-order --host hev-node-EXAMPLE
```

An order is markdown with YAML frontmatter naming name, host, repo, base, harness, model and reply, followed by concrete Acceptance items. Use `validation: none`; requesting validation is refused. The host derives the requester from your key.

Check `hev dispatch ls` for finished runs; a run's report and PR are in `show` and `logs`. `say` always queues a message in the run's inbox and never restarts an ended run.

Members cannot attach, rework, amend, retry, clean, validate, start orchestrators or expedite, read other senders' runs, or change which repositories the host admits. Ask the host owner for those. If a command is refused with a permission error, report it rather than retrying another way.
