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

The host can't reach this machine, so it holds a notice when each of your runs finishes, fails or stops. A running hevd (`hev up`) pulls them every minute; fetch them by hand with `hev dispatch notices pull` (add `--host hev-node-EXAMPLE` for one host, `--watch` to keep pulling every minute); a notice goes to the run's `reply` session inbox if it has one here, otherwise it is printed. `hev dispatch ls` also shows run state; a run's report and PR are in `show` and `logs`. `say` always queues a message in the run's inbox and never restarts an ended run.

Before sending, check the repository is admitted. Every member can list admitted repositories; only a member granted the `repos` capability can add or remove one (`add` clones it into the host workspace; dispatch it by its directory name). Without that grant, ask the host owner.

```sh
hev admission ls --host hev-node-EXAMPLE
hev admission add https://example.test/ExampleOrg/widget.git --host hev-node-EXAMPLE
hev admission remove https://example.test/ExampleOrg/widget.git --host hev-node-EXAMPLE
```

Members cannot attach, rework, amend, retry, clean, validate, start orchestrators or expedite, or read other senders' runs. Ask the host owner for those. If a command is refused with a permission error, report it rather than retrying another way.
