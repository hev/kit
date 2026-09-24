# hev kit

Get your agency back.

hev kit keeps coding-agent transcripts searchable in infrastructure you
control. It reads harness-native session files, normalizes them into one trace
model, and writes content-addressed chunks to a [hev layer](https://hevlayer.com)
namespace. Search is hybrid and runs in the store; kit computes no vectors and
carries no reranker.

See [RFC 0002](docs/rfcs/0002-direct-to-layer.md) for the direct-to-Layer
architecture and [RFC 0003](docs/rfcs/0003-trace-model.md) for the trace model.

## Install

```bash
brew install hev/tap/kit
```

This installs the `hev` binary (macOS). From source: `go install
github.com/hev/kit/cmd/hev@latest`.

## Before you run it: sensitive data

Coding agent transcripts hold whatever the agent saw: keys pasted into a
prompt, secrets on a command line, file contents, customer data. `hevd`
indexes every Claude Code session under `~/.claude/projects` and every Codex
session under `~/.codex/sessions` (prompts, replies and tool calls; tool
results only if you run `hev index --tier all`) and writes them to your namespace as they are.

kit does no redaction and no per-project filtering yet. The `[projects]`
allow and deny lists in the config are read but not applied to indexing, so
do not rely on them to keep a repository out. Run kit on a machine whose
sessions you are comfortable storing in that namespace, and treat the
namespace and your turbopuffer key with the same care as the transcripts
themselves.

## Quick start

You need Docker running and a [turbopuffer](https://turbopuffer.com) API key:

```console
$ export TURBOPUFFER_API_KEY=tpuf_...
$ hev up
  ✓ docker running
  ✓ layer-gateway:edge running on :8080
  ✓ turbopuffer key accepted, archiving to namespace hev-traces
  ✓ hevd installed, first scan started
  ✓ dashboard running http://127.0.0.1:8099

$ hev find "why did the preflight fail"
```

`hev up` runs two containers from a Compose file built into the binary: the
[hev layer](https://hevlayer.com) gateway (community edition) in front of your
turbopuffer account, and the kit dashboard reading through it. It checks the
key through the gateway, writes `~/.hev/config.toml` to point at the stack,
and installs the capture daemon (`hevd`) under launchd on the host, where the
transcripts are. Your archive is a namespace in your own turbopuffer account.

Run again, `hev up` reports what is already running and changes nothing; the
key is kept in the config, so nothing needs to be exported the second time.
Export a new `TURBOPUFFER_API_KEY` and run `hev up` to rotate it. `hev down`
stops the containers and unloads the daemon, and deletes nothing: the archive
stays in turbopuffer. `up` will not repoint a config that already names a
hosted Layer. See [RFC 0006](docs/rfcs/0006-up-command.md).

To use a hosted Layer instead:

```bash
hev init       # enter Layer endpoint, API key, and namespace
hev d          # index now, then keep the archive current
hev find "why did the preflight fail"
```

`hev d` scans immediately and then at `capture.scan_interval`. A transcript
that is still growing gets a new size/mtime signature and is indexed again.
Unchanged units are skipped. A unit is recorded as complete only after every
chunk lands, so an unreachable target leaves it pending and the next successful
cycle catches up without a manual `hev index` run.

The current implementation re-reads and re-chunks an entire changed session.
That is deliberately simple and correct for live transcripts; tail-only
chunking is deferred until long-session measurements justify the extra state
and chunk-boundary complexity.

`hev index` remains available for a manual run:

```bash
hev index
hev index --tier all            # include bulky tool results
hev index --read-side --force --workers 4  # bounded parallel backfill, no embedding
hev find --plan my-plan "what did the worker try"
```

`--workers` defaults to 1 and accepts 1–8. Values above 1 require `--read-side`
and cannot be combined with `--summarize`. Failed units stay retryable; the
final report lists any read or write errors. Keep a backfill supervised and
check that report before treating the run as complete.

## Configuration

Capture reads `~/.hev/config.toml`. `hev init` writes:

```toml
[layer]
endpoint = "https://gcp-us-central1.turbopuffer.com"
api_key = "..."
namespace = "hev-traces"

[capture]
scan_interval = "5m"
```

`hev up` writes the local equivalent, plus the block that makes the image
pins, the host ports and the Compose project name a config value each:

```toml
[layer]
endpoint = "http://127.0.0.1:8080"
api_key = "tpuf_..."           # your turbopuffer key: the gateway's bearer and its upstream credential
namespace = "hev-traces"
store = "turbopuffer"

[local]
image = "hevlayer/layer-gateway:edge"
port = 8080
project = "hev-kit"
kit_image = "hevlayer/kit:0.1.0"   # the dashboard; a release binary pins its own version
serve_port = 8099
```

`hev up` rewrites `config.toml` as a whole (atomically, mode 0600): every key
is carried over, comments are not. `hev down` on a config with no `[local]`
block — one `hev up` never wrote — stops the project's containers and leaves
launchd jobs alone.

`HEV_LOCAL_IMAGE`, `HEV_LOCAL_PORT`, `HEV_LOCAL_PROJECT`,
`HEV_LOCAL_KIT_IMAGE` and `HEV_LOCAL_SERVE_PORT` override the block, and are
recorded in it by `hev up`. A busy port is an error naming the override, never
a silent move.

`LAYER_ENDPOINT`, `LAYER_API_KEY`, and `LAYER_NAMESPACE` override the file.
`LAYER_EMBED_MODEL` defaults to `qwen/qwen3-embedding-8b`; choose it before the
first write because an existing namespace cannot be re-embedded. Old configs
containing only S3 bucket settings fail with instructions to run `hev init` and
use `hev migrate` for an existing archive.

## Commands

```text
hev up                  Start the gateway and dashboard in Docker, and the daemon
hev up --no-dashboard   Run the gateway and daemon only
hev down                Stop the containers and unload the daemon
hev init                Configure a hosted Layer archive target
hev d                   Run continuous indexing
hev s                   Show last run, units indexed, and last error
hev stop                Stop the daemon
hev daemon install      Install the launchd service
hev daemon uninstall    Remove the launchd service
hev config show         Print effective config with secrets redacted
hev index               Run indexing once
hev find <query>        Search the archive
hev ls                  List sessions
hev trace <session-id>  Show a session
```

The local spool and ledger remain the transport durability layer for legacy
envelopes and migration. They are not the searchable archive and are not
discarded during this transition. Transcript indexing uses its own unit state;
failed units are retried and content-addressed chunk IDs make partial rewrites
idempotent.

On macOS, `hev daemon install` installs `com.hev.hevd` and starts an index
cycle immediately, then repeats at `capture.scan_interval` (default five
minutes). It opens no listening port. `hev daemon status` reports the local
process, launchd job, and last completed cycle from `~/.hev/daemon-status.json`.
Reinstall replaces only this job; stop unloads it so KeepAlive cannot restart it.

launchd reads the config selected by `HEV_CONFIG` at installation. Put Layer
settings in that file; interactive shell exports are not inherited.
