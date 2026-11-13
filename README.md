# hev kit

hev kit is what you need to make your machine part of a software factory.
It's one binary with the tools a factory needs: a daemon that collects
traces, dispatch that sends work to your hosts, and an admission controller
that decides which repos a host accepts (and probably tools and skills over
time). `brew install hev` gets you everything you need to run a factory. To
get the most out of the factory you'll want headless machines locally or in
the cloud.

## Install

On macOS or Linux with Homebrew, the tap command works now:

```sh
brew install hev/tap/hev
```

Bare `brew install hev` needs a formula in homebrew-core. The tap formula
installs the binary, which includes bundled agent skills. Existing `hev/tap/kit` users can
upgrade that compatibility formula, which depends on `hev`.

Linux and Windows binaries are on [GitHub Releases](https://github.com/hev/kit/releases).
Extract the archive for your OS and architecture and put the binary on your PATH.
0.6 is not tagged yet; build from main for the factory features described here
(Go is required):

```sh
go install github.com/hev/kit/cmd/hev@main
```

See the [Windows guide](docs/windows.md) for capture service setup. With no
key, `hev up` keeps traces in files on this machine and needs no Docker.

## Five-minute quickstart

Start the local archive and capture daemon, then check it:

```sh
hev up
hev s
```

Register a host in `~/.config/hev/dispatch.toml`. This example uses your own
machine; it needs Git, tmux, the Codex CLI, and an existing clone at
`~/workspace/sample` with an `origin` remote. Use a repository you want an agent
to work on, and replace `sample` below with its directory name.

```toml
cap = 2
workspace = "~/workspace"

[hosts.here]
local = true
```

Save an order as `order.md`:

```markdown
---
name: explain-project
host: here
repo: sample
base: main
harness: codex
validation: none
---
Read the project and write a short architecture guide in ARCHITECTURE.md.
Do not push or open a PR.

## Acceptance
- The guide explains the entry points and how to run the project.
```

Dispatch it, follow its log, and inspect the result:

```sh
hev dispatch order.md
hev dispatch logs explain-project -f
hev dispatch show explain-project
```

Dispatch creates an isolated Git worktree and keeps the order, status, report,
and logs under `~/dispatch/explain-project/`. Once capture has indexed the
session, search its trace:

```sh
hev traces search 'architecture' --order explain-project
```

The [dispatch guide](docs/dispatch.md) covers remote hosts, inline tasks,
notifications, validation, and rework. The [online docs](https://hevkit.com/docs)
walk through the factory setup.

## Dispatch

Send an order to a configured host with `hev dispatch`. Each order names its
repository, harness, and acceptance criteria. Dispatch reserves capacity,
creates a worktree, starts the agent, and records its result. It works without
the capture daemon. Follow work with `hev dispatch logs`, inspect it with
`hev dispatch show`, and send instructions with `hev dispatch say`. Replace an order with
`hev dispatch amend` while keeping its name, branch, and PR.
An order can name a context recipe (`context: PATH`); dispatch builds an
explicit, reproducible context bundle at launch, gives it to the worker, and
records its digest on the run. `hev context` assesses, builds, compares and
reviews bundles on their own. See [dispatch](docs/dispatch.md) and
[composable context](docs/context.md).

## Admission

The receiving host decides which repositories it accepts. Set top-level `cap`
and `[admission] repos` in that host's dispatch configuration. Repository policy
matches the clone's origin remote before fetch or worktree creation, and is
checked again before rework or an ended-run relaunch. The target reads policy
from its own default configuration (or its own `HEV_DISPATCH_CONFIG`); a sender
cannot select a different admission policy. Omitted `repos` admits all, and an
empty list admits none. A per-route host `cap` can lower the sender ceiling;
the target enforces its own capacity. Upgrade receiving hosts to
0.6 before relying on this policy. See [repository admission](docs/dispatch.md#repository-admission).
Invited teammates can add and remove repositories themselves only when the owner sets
`[admission] add_from` and grants them `repos`; see
[teammate-added repositories](docs/nodes.md#teammate-added-repositories).

## Peers

`hev peers` discovers agent sessions in tmux and dispatch runs using native Go;
no external peers program is required. Use `hev peers read` to see what another
agent is doing and `hev peers send` to coordinate before touching shared work.
See [peers](docs/peers.md).

## Board

`hev board` gives agents a shared place for findings and decisions that should
survive a session. Search it before repeating an investigation, then leave what
you learned for the next agent. See [the board](docs/board.md).

## Traces

Hevd can also run an opt-in [encrypted membership cluster](docs/cluster.md)
with liveness and shared usage facts.

The daemon captures coding sessions into a searchable archive. With no hev
layer key, `hev up` starts nothing in Docker: capture writes to a file archive
under `~/.hev/archive/`, and search over it matches words, not meaning (see
[the trace archive](docs/trace-archive.md)). To use the shared hosted archive
on [hev layer](https://hevlayer.com), export `HEVLAYER_API_KEY` and run
`hev up`; capture writes to hev layer and `hev open` opens the public hosted
dashboard. Saved targets are reused; select `--store`
explicitly to move an existing archive setup. `HEVLAYER_ENDPOINT` and `HEVLAYER_NAMESPACE` select
the gateway and archive. See [hosted setup and advanced stores](docs/hevlayer.md).
Capture reads Claude Code, Codex, [PI](docs/native-pi.md),
[OpenCode](docs/native-opencode.md) and dot sessions from their usual local
stores. Use `hev traces search` to find prior work, `hev traces show` to read a session,
and `hev traces ls` to list captured sessions. `hev d` starts capture and
`hev s` reports its status. `hev down` stops capture and local services and preserves
the archive.

Secret scrubbing is enabled by default, but source transcripts remain raw and
scrubbing cannot cover every kind of sensitive data. Treat transcripts, the
archive, and configuration as sensitive. Read [redaction and archive upgrades](docs/archive-redaction.md)
before upgrading an existing archive. See [instruction capture](docs/skill-capture.md)
and [session enrichment](docs/session-backfill.md) for more ways to use the record.

## Secrets

Use `hev secrets run --ref TOKEN=op://example/service/token -- command` to
resolve service-account secrets into a child's environment,
`x=$(hev secrets read op://example/service/token)` for one value in a shell, and
`eval "$(hev secrets env --ref TOKEN=op://...)"` for several. The local daemon
batches and caches reads in memory (five minutes by default); `--ttl TOKEN=0s`
requires a fresh read. Every real 1Password request is logged by label in
`~/.hev/secrets/op-reads.log`, and `hev secrets status` shows hits, misses,
requests and the account quota without values. Read
[how a host reads secrets](docs/secrets.md) for the procedure and
[the secrets RFC](docs/rfcs/0009-secrets.md) for the security boundary and
rate-limit behavior.

## Headless machines

Add machines in a closet or in the cloud to keep work running while your laptop
is away. Remote hosts need SSH access, `hev`, Git, a configured agent harness,
and repository clones; Unix hosts also need tmux. Each host owns its capacity
and repository admission policy. Capture must be configured separately on each
host. See [host configuration](docs/dispatch.md) and [headless setup](https://hevkit.com/docs/headless).

More guides live in [docs/](docs/) and at [hevkit.com/docs](https://hevkit.com/docs).

A machine that only sends work needs no daemon: `hev skills setup` installs the
bundled agent skills for Claude Code, Codex, pi and OpenCode, records the
machine's skills role and syncs team sources, and starts nothing. `hev up` runs
the same bundled install. Teams can install their own pinned skills alongside
bundled skills using `[[skills.sources]]` in the HEV config and
`hev skills sync`. See
[skill sources, targeting, and scheduling](docs/rfcs/0012-skill-sources.md).
