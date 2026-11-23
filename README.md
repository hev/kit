<p align="center">
  <img src="docs/assets/hev-head.png" alt="hev kit" width="120" height="120" />
</p>

# hev kit

hev kit is a **meta-harness** for Claude Code, Codex, Antigravity (agy), Pi,
and OpenCode. It works across harnesses, sessions, and machines and ships as
one distributed `hev` binary for macOS, Linux, and Windows.

A workspace binds subscriptions and authentication to a home folder and a
private network with a cluster. Its machines provide shared tools for traces,
board, peers, dispatch, cron, orchestration, secrets, and cluster management.
Start on one machine and add local or cloud hosts as you need capacity. Each
host controls which repositories it accepts.

Starter Kit runs locally without a hosted account. Pro adds hosted services
for shared storage and search.

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
0.6 is not tagged yet; build from main for the features described here
(Go is required):

```sh
go install github.com/hev/kit/cmd/hev@main
```

See the [Windows guide](https://hevkit.com/docs/windows) for capture service setup. With no
key (Starter Kit), `hev up` keeps traces in files on this machine, searches
them with grep and needs no Docker or database.

## Five-minute quickstart

Install with Homebrew and start kit:

```sh
brew tap hev/tap
brew trust hev/tap
brew install hev/tap/hev
hev up
```

On a new machine, `hev up` runs `hev onboard`: confirm your detected harnesses
and workspace (default `~/workspace`), and it creates `~/.hev/` (`traces`,
`board`, `orders`, `peers`, `hooks`, `evidence`), writes a local
`~/.config/hev/dispatch.toml` naming this machine `here`, installs bundled
agent skills, and starts `hevd` and the local dashboard
(`http://127.0.0.1:8099`). Check status any time with `hev s`.

Open your harness:

```sh
hev kit                # built-in harness (free routes on Starter Kit, no key required)
```

Or use your existing harness—`hev up` already installed kit's bundled skills
(re-run `hev skills setup` any time):

```sh
hev skills setup
claude                 # or codex, agy, pi, opencode
```

Then ask your harness to dispatch your first item (for example: *"Dispatch an
order to add a short Development section to README.md in `sample`, commit on a
branch, and don't push"*). On the first run, the agent walks you through
repository admission, creates an isolated Git worktree, follows the run, and
shows you the receipt.

Prefer driving from the shell? Dispatch an order and search its trace directly:

```sh
hev dispatch order.md
hev dispatch logs explain-project -f
hev dispatch show explain-project
hev traces search 'architecture' --order explain-project
```

The [quickstart](https://hevkit.com/docs/quickstart),
[dispatch guide](https://hevkit.com/docs/dispatch), and
[online docs](https://hevkit.com/docs) cover admission, orders, validation,
rework, and clusters.

## Dispatch

Send an order to a configured host with `hev dispatch`. Each order names its
repository, harness, and acceptance criteria. Dispatch reserves capacity,
creates a worktree, starts the agent, and records its result. It works without
the capture daemon. Follow work with `hev dispatch logs`, inspect it with
`hev dispatch show`, and send instructions with `hev dispatch say`. Replace an order with
`hev dispatch amend` while keeping its name, branch, and PR.
Review a run's branch with `hev review`, which opens it in
[hunk](https://hunk.dev) with the worker's and reviewers' anchored notes, and
send your own notes back with `hev review send`; see [review](https://hevkit.com/docs/review).
An order can name a context recipe (`context: PATH`); dispatch builds an
explicit, reproducible context bundle at launch, gives it to the worker, and
records its digest on the run. `hev context` assesses, builds, compares and
reviews bundles on their own. See [dispatch](https://hevkit.com/docs/dispatch) and
[composable context](https://hevkit.com/docs/context).

## Admission

The receiving host decides which repositories it accepts. Set top-level `cap`
and `[admission] repos` in that host's dispatch configuration. Repository policy
matches the clone's origin remote before fetch or worktree creation, and is
checked again before rework or an ended-run relaunch. The target reads policy
from its own default configuration (or its own `HEV_DISPATCH_CONFIG`); a sender
cannot select a different admission policy. Omitted `repos` admits all, and an
empty list admits none. A per-route host `cap` can lower the sender ceiling;
the target enforces its own capacity. Upgrade receiving hosts to
0.6 before relying on this policy. See [repository admission](https://hevkit.com/docs/admission).
Invited teammates can add and remove repositories themselves only when the owner sets
`[admission] add_from` and grants them `repos`; see
[teammate-added repositories](https://hevkit.com/docs/nodes#teammate-added-repositories).

## Peers

`hev peers` discovers agent sessions in tmux and dispatch runs using native Go;
no external peers program is required. Use `hev peers read` to see what another
agent is doing and `hev peers send` to coordinate before touching shared work.
See [peers](https://hevkit.com/docs/peers).

## Native FOH harness (optional)

`hev foh` is an optional front-of-house harness built into `hev`: a lean Go
agent loop routed through OpenRouter in a TUI with a model picker (`Ctrl+M`),
mid-thread model switching, a composable system prompt and first-class
`hev_dispatch`, `hev_peers`, `hev_board`, `hev_traces` and `hev_status` tools
rendered as cards. Tools run YOLO (no approval prompts); back-of-house work
stays on `claude`, `codex` and `agy`. See [hev foh](https://hevkit.com/docs/foh-harness).

## Board

`hev board` gives agents a shared place for findings and decisions that should
survive a session. Search it before repeating an investigation, then leave what
you learned for the next agent. On Starter Kit (no key) each machine keeps its
own board in files under `~/.hev/board`, `hev board post`, `search`, `read` and
`reply` work without a server, and search matches words; Club Kit and Pro Kit
share one board across machines and also match meaning. See
[the board](https://hevkit.com/docs/board).

## Evidence

`hev evidence` keeps proof packages from validation, browser-use,
computer-use and spike runs: screenshots, short videos (MP4, WebM, MOV up to
60s) and text, with claims such as `pass` or `fail`. On Starter Kit each
package is a folder under `~/.hev/evidence` with a `manifest.json`; files are
checked by their bytes, text is scrubbed of secrets before it is written, and
`hev evidence search` matches words. `hev s` reports what each surface uses
on disk under `~/.hev`. See [evidence](https://hevkit.com/docs/evidence).

## App traces

`hev collect` captures a host app's own agent traces (Vercel AI SDK spans over
OTLP/HTTP, sent with the `@hevmind/trace` package) and serves the app's agent
tools a search over each user's past turns and a memory board. With no Layer
key it keeps them in the file archive and searches with grep; with a key, in
hev layer. See [app traces and memory](https://hevkit.com/docs/app-traces).

## Traces

Hevd can also run an opt-in [encrypted membership cluster](https://hevkit.com/docs/cluster)
with liveness and shared usage facts.

The daemon captures coding sessions into a searchable archive. With no hev
layer key (Starter Kit), `hev up` starts nothing in Docker: capture writes
redacted sessions to a file archive under `~/.hev/traces/`, `hev serve` runs
as a service for the dashboard on `http://127.0.0.1:8099` (two tabs: Traces,
and a read-only Board over `~/.hev/board`), and search is grep
(`rg` when it is installed): a hit contains every word you type, not
something that means the same (see [the trace archive](https://hevkit.com/docs/trace-archive)).
A config from the retired local Postgres lane is moved to files on the next
`hev up`, exporting its rows when its containers start. To use the shared hosted archive
on [hev layer](https://hevlayer.com), export `HEVLAYER_API_KEY` and run
`hev up`; capture writes to hev layer and `hev open` opens the public hosted
dashboard. Saved targets are reused; select `--store`
explicitly to move an existing archive setup. `HEVLAYER_ENDPOINT` and `HEVLAYER_NAMESPACE` select
the gateway and archive. Bring-your-own Turbopuffer is hosted-only. See
[hosted setup](https://hevkit.com/docs/kit-and-layer).
Capture reads Claude Code, Codex, [Pi](https://hevkit.com/docs/native-pi),
[OpenCode](https://hevkit.com/docs/native-opencode), Antigravity (`agy`) and dot sessions from their usual local
stores. Use `hev traces search` to find prior work, `hev traces show` to read a session,
`hev traces ls` to list captured sessions, and [`hev traces stats`](https://hevkit.com/docs/traces-stats)
to count skill loads, tool use and dispatch health across them. `hev d` starts capture and
`hev s` reports its status. `hev down` stops capture and the dashboard service and
preserves the archive.

Secret scrubbing is enabled by default, but source transcripts remain raw and
scrubbing cannot cover every kind of sensitive data. Treat transcripts, the
archive, and configuration as sensitive. Read
[redaction and archive upgrades](https://hevkit.com/docs/archive-redaction)
before upgrading an existing archive. See [instruction capture](https://hevkit.com/docs/skill-capture)
and [trace backfill](https://hevkit.com/docs/traces-stats#backfill) for more ways to use the record.

## Secrets

Use `hev secrets run --ref TOKEN=op://example/service/token -- command` to
resolve service-account secrets into a child's environment,
`x=$(hev secrets read op://example/service/token)` for one value in a shell, and
`eval "$(hev secrets env --ref TOKEN=op://...)"` for several. The local daemon
batches and caches reads in memory (five minutes by default); `--ttl TOKEN=0s`
requires a fresh read. Every real 1Password request is logged by label in
`~/.hev/secrets/op-reads.log`, and `hev secrets status` shows hits, misses,
requests and the account quota without values. Read
[how a host reads secrets](https://hevkit.com/docs/secrets) for the procedure and
the secrets design decision on `hev board` for the security boundary and
rate-limit behavior.

## Headless machines

Add machines in a closet or in the cloud to keep work running while your laptop
is away. Remote hosts need SSH access, `hev`, Git, a configured agent harness,
and repository clones; Unix hosts also need tmux. Each host owns its capacity
and repository admission policy. Capture must be configured separately on each
host. See [host configuration](https://hevkit.com/docs/dispatch) and [headless setup](https://hevkit.com/docs/hosts).

User guides are published at [hevkit.com/docs](https://hevkit.com/docs). The site
source lives in `hev/kit-cloud/site/`. Find decisions, design memos, runbooks
and learnings on `hev board`, proof packages in `hev evidence`, and proposals
and specs in dispatch orders (`hev dispatch`, stored under `~/.hev/orders`).
See [release notes](releases/) and the [Layer schema policy](schemas/layer/).

A machine that only sends work needs no daemon: `hev skills setup` installs the
bundled agent skills for Claude Code, Codex, pi and OpenCode, records the
machine's skills role and syncs team sources, and starts nothing. `hev up` runs
the same bundled install. Teams can install their own pinned skills alongside
bundled skills using `[[skills.sources]]` in the HEV config and
`hev skills sync`. See the skill sources design decision on `hev board`.

### Skills shelf

Popular skill packs go on a short, pinned shelf instead of into every
session. A pack is installed at a reviewed commit but stays inactive until a
dispatch run enables some of its skills with `skills: [pack/skill, ...]` in
the order. See [the skills shelf](https://hevkit.com/docs/skills-shelf).

```sh
hev skills add cursor/plugins//pstack --kind mode   # pin and install, not active
hev skills shelf                                    # packs, pins, licences, skills
hev skills shelf diff pstack --patch                # upstream changes since the pin
hev skills shelf bump pstack --to SHA               # move the pin to a reviewed commit
hev skills search "write the failing test first"    # the skills collection
```
