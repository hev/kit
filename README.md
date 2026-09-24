# hev kit

**Take back your agency.** hev kit makes your coding agent traces searchable
via a hybrid search system built on [turbopuffer](https://turbopuffer.com) and
[hev layer](https://hevlayer.com). Install the hev daemon on as many machines
as you want to share and search your traces. The tour is at
[hev.dev/kit](https://hev.dev/kit/).

> [!CAUTION]
> **Your transcripts may contain sensitive data.** Coding agent sessions hold
> whatever the agent saw: keys pasted into a prompt, secrets on a command line,
> file contents, customer data. hevd indexes every coding agent session on each
> machine it runs on, including prompts, replies and tool calls, and writes
> them to your turbopuffer namespace as they are. kit does not redact anything
> or filter by project yet. Run it on a machine whose sessions you are
> comfortable storing there, and treat the namespace and your key with the same
> care as the transcripts.

## Quick start

You need macOS, Docker running and a [turbopuffer](https://turbopuffer.com)
API key.

```bash
brew install hev/tap/kit
export TURBOPUFFER_API_KEY=tpuf_...
hev up
hev query "why did the preflight fail"
```

`hev up` starts the hev layer gateway and the kit dashboard in Docker, writes
`~/.hev/config.toml`, installs the capture daemon (`hevd`) under launchd, and
installs the [agent skills](#agent-skills) for Claude Code and Codex.
The dashboard is at http://127.0.0.1:8099. The key is saved in the config, so
running `hev up` again needs nothing exported. `hev down` stops everything and
leaves the archive in turbopuffer.

When every step has passed, `up` ends on hevd and a summary (in a terminal
only; piped output keeps just the `✓` lines):

```text
   ▄▀▄   ▄▀▄    hevd is up, capturing sessions on this machine.
  ▐████████▌    dashboard  http://127.0.0.1:8099
  ▐█ ▀  ▀ █▌    search     hev query "why did the preflight fail"
   ▀██▄▄██▀ ψ   archive    <(°O°)> turbopuffer · hev-traces
    ▐█  █▌      stop       hev down
```

To build from source, run `go install github.com/hev/kit/cmd/hev@latest`.

## Configuration

`hev up` writes `~/.hev/config.toml`:

```toml
[layer]
endpoint = "http://127.0.0.1:8080"
api_key = "tpuf_..."
namespace = "hev-traces"
store = "turbopuffer"

[local]
image = "hevlayer/layer-gateway:0.6.0"
port = 8080
project = "hev-kit"
kit_image = "hevlayer/kit:0.1.1"
serve_port = 8099

[capture]
scan_interval = "5m"
```

`HEV_LOCAL_IMAGE`, `HEV_LOCAL_PORT`, `HEV_LOCAL_PROJECT`, `HEV_LOCAL_KIT_IMAGE`
and `HEV_LOCAL_SERVE_PORT` override the `[local]` block, and `hev up` records
them there. If a port is busy, `hev up` stops and names the variable to set.

To archive to a hosted Layer instead of the local gateway, run `hev init` and
enter its endpoint, API key and namespace. `LAYER_ENDPOINT`, `LAYER_API_KEY`
and `LAYER_NAMESPACE` override the file for commands run in your shell. The
daemon runs under launchd and reads only the file.

The hev layer gateway sends anonymous telemetry: a started event and a daily
heartbeat with a random instance id, the gateway version, the store kind and
feature counts. It never sends queries, results or transcripts. Export
`DO_NOT_TRACK=1` before `hev up` to turn it off.

## Commands

```text
hev up                  Start the gateway, dashboard and daemon
hev up --no-dashboard   Start the gateway and daemon only
hev down                Stop the containers and unload the daemon
hev query <query>       Search the archive (alias: find)
hev ls                  List sessions from the last 24h (--since 5d to widen)
hev trace <session-id>  Show a session
hev                     Browse traces interactively
hev s                   Daemon status: last run, units indexed, last error
hev index               Index once (--tier all includes tool results)
hev init                Configure a hosted Layer
hev config show         Print the effective config, secrets redacted
```

## Agent skills

`skills/` holds skills that teach Claude Code and Codex to use the archive
instead of their built-in session history. `hev-query` fans a question out
into several phrasings, gathers and dedupes the hits, and reads a window of
each matching session. `hev up` installs them into `~/.claude/skills` and
`~/.codex/skills` for each harness installed on the machine, and replaces them
on upgrade. To work on a skill, symlink it from a clone instead: `hev up`
leaves a symlinked skill alone.

```bash
ln -sfn "$PWD/skills/hev-query" ~/.claude/skills/hev-query
```
