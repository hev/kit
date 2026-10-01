# hev kit

**Take back your agency.** hev kit makes your coding agent traces searchable
via a hybrid search system built on [hev layer](https://hevlayer.com). It is
free and local by default: `hev up` runs Layer's Postgres store and a CPU
embedding model on your machine, with no account and no key.
[turbopuffer](https://turbopuffer.com) is the optional hosted lane. Install the
hev daemon on as many machines as you want to share and search your traces.
The tour is at [hev.dev/kit](https://hev.dev/kit/).

> [!CAUTION]
> **Your transcripts may contain sensitive data.** Secret scrubbing is enabled
> by default in CE and the hosted lane, before chunks, dashboard rows or summary
> model input are built. It covers gitleaks v8.24.3 text patterns (including cloud
> and GitHub keys), Slack `xox*`, `sk-` and `tpuf_` tokens, PEM private keys and
> certificates, JWTs, URL user/password credentials, `Authorization:` values,
> and `KEY=value` assignments with values of at least 20 characters and Shannon
> entropy of at least 3.5. Provider rules retain keyword and entropy thresholds;
> transcript paths, example allowlists and `gitleaks:allow` comments do not exempt
> matches. Overlapping matches are replaced and counted once.
>
> Replacements are `[REDACTED:<rule>#<fingerprint>]`. The searchable fingerprint
> is HMAC-SHA256 truncated to 64 bits, keyed by a random per-install 32-byte
> `capture.redact_salt` persisted in mode-0600 `~/.hev/config.toml` (or
> `HEV_CONFIG`). Keep that salt stable and private: changing it changes future
> fingerprints, and different installations produce different fingerprints.
> `hev s` reports per-rule replacements from the last daemon scan, including
> attempted writes; these counts are not an archive inventory.
>
> The first enabled post-upgrade scan of each Claude/Codex source root deletes
> its old session chunks, blocks, prompts and summaries, verifies deletion, and
> rebuilds all tiers from scrubbed source transcripts, even when signatures are
> unchanged. Cleanup retries after interruption; missing sources can mean loss
> of historical sessions, and ambiguous ownership stops the upgrade. Stop older
> writers before upgrading. See [archive upgrade handling](docs/archive-redaction.md)
> for scope, failures and restoring raw backups.
>
> `[capture] redact = false` explicitly opts out of both scrubbing and upgrade
> cleanup; re-enabling scrubbing triggers cleanup again. **Source transcripts
> remain raw.** Arbitrary passwords, customer data, encoded or split secrets,
> and unsupported formats are not guaranteed coverage. Treat the archive, source
> files, config salt and access keys as sensitive even with scrubbing enabled.

## Quick start

You need macOS and Docker running.

```bash
brew install hev/tap/kit
hev up
hev query "why did the preflight fail"
```

`hev up` starts the hev layer gateway, its Postgres store, the CPU embedding
sidecar and the kit dashboard in Docker, writes `~/.hev/config.toml`, installs
the capture daemon (`hevd`) under launchd, and installs the
[agent skills](#agent-skills) for Claude Code and Codex. The dashboard is at
http://127.0.0.1:8099. The gateway embeds each chunk with
`BAAI/bge-small-en-v1.5` on your CPU, and `hev query` gets hybrid search,
semantic and BM25, fused by the gateway. Nothing leaves the machine and
nothing costs anything. `hev down` stops everything and leaves the archive in
its Docker volume.

On the free lane every command and the whole dashboard work, including its
session list, stats, search, filter by tool and the session summaries
`hev index --summarize` writes. An
archive started on kit v0.3.0 stored the session list's tool names as text;
after an upgrade, `hev up` (or hevd, when it starts on the new binary)
rewrites that namespace with array attributes, once, without indexing
anything again.

### The hosted lane: turbopuffer

With a [turbopuffer](https://turbopuffer.com) API key, the gateway runs in
front of your turbopuffer account instead, embeds with
`qwen/qwen3-embedding-8b`, and every command works:

```bash
export TURBOPUFFER_API_KEY=tpuf_...
hev up
```

The key is saved in the config, so running `hev up` again needs nothing
exported, and `hev down` leaves the archive in turbopuffer. A machine stays on
the store its first `hev up` chose; `hev up --store turbopuffer` (or
`--store pgvector`) moves it, starting the new archive empty and indexing
every transcript into it again.

When every step has passed, `up` ends on hevd and a summary (in a terminal
only; piped output keeps just the `✓` lines):

```text
   ▄▀▄   ▄▀▄    hevd is up, capturing sessions on this machine.
  ▐████████▌    dashboard  http://127.0.0.1:8099
  ▐█ ▀  ▀ █▌    search     hev query "why did the preflight fail"
   ▀██▄▄██▀ ψ   archive    postgres, on this machine · hev-traces
    ▐█  █▌      edition    hev layer community · pro: hev pro
                stop       hev down
```

To build from source, run `go install github.com/hev/kit/cmd/hev@latest`.

## Configuration

`hev up` writes `~/.hev/config.toml`:

```toml
[layer]
endpoint = "http://127.0.0.1:8080"
api_key = "local"
namespace = "hev-traces"
store = "pgvector"

[local]
image = "hevlayer/layer-gateway:0.7.3"
embed_image = "hevlayer/layer-embed:0.7.3"
port = 8080
project = "hev-kit"
kit_image = "hevlayer/kit:0.1.1"
serve_port = 8099

[capture]
scan_interval = "5m"
```

On the hosted lane `api_key` is your turbopuffer key, `store` is
`turbopuffer`, and there is no `embed_image`.

`HEV_LOCAL_IMAGE`, `HEV_LOCAL_EMBED_IMAGE`, `HEV_LOCAL_PORT`,
`HEV_LOCAL_PROJECT`, `HEV_LOCAL_KIT_IMAGE` and `HEV_LOCAL_SERVE_PORT` override
the `[local]` block, and `hev up` records them there. `LAYER_EMBED_MODEL`
overrides the embedding model for either store; changing it on an existing
archive means indexing it again into a new namespace. If a port is busy, `hev up` stops and names the variable to set.

To archive to a hosted Layer instead of the local gateway, run `hev init` and
enter its endpoint, API key and namespace. `LAYER_ENDPOINT`, `LAYER_API_KEY`
and `LAYER_NAMESPACE` override the file for commands run in your shell. The
daemon runs under launchd and reads only the file.

The hev layer gateway sends anonymous telemetry: a started event and a daily
heartbeat with a random instance id, the gateway version, the store kind and
feature counts. The gateway `hev up` starts also names kit as the
distribution that started it, with kit's version, and says nothing else about
you or your data. It never sends queries, results or transcripts. Export
`DO_NOT_TRACK=1` (or `LAYER_TELEMETRY=off`) before `hev up` to turn it off.

## Commands

```text
hev up                  Start the gateway, store, dashboard and daemon (free, local)
hev up --no-dashboard   Start the gateway, store and daemon only
hev up --store <kind>   Move the archive to pgvector or turbopuffer
hev down                Stop the containers and unload the daemon
hev query <query>       Search the archive (alias: find)
hev ls                  List sessions from the last 24h (--since 5d to widen)
hev trace <session-id>  Show a session
hev                     Browse traces interactively
hev s                   Daemon status: last run, units indexed, last error
hev index               Index once (--tier all includes tool results)
hev init                Configure a hosted Layer
hev pro                 The gateway's edition, and what hev layer pro adds
hev pro trial           Open the hev layer pro trial signup
hev config show         Print the effective config, secrets redacted
```

## hev layer pro

`hev up` runs the community edition of the hev layer gateway. hev layer pro
is the licensed edition: scoped, revocable keys per machine or teammate,
functions on write, search history, cost views and agents. `hev pro` asks the
gateway you are pointed at which edition it is (`GET /v2/license`, answered
offline from its key) and shows what pro adds; `hev s` and the `hev up`
summary carry the same edition line, and warn when a license is within two
weeks of its end. `hev pro trial` opens the trial signup.

kit also prints an occasional hint, on stderr, when something you just did is
what pro is for (for example, sessions from several machines sharing one
Turbopuffer key). Hints only appear at an interactive terminal, never inside
Claude Code, Codex or CI, at most once a week each, and never change what a
command does. `HEV_NO_HINTS=1` turns them off.

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
