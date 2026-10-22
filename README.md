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
> by default in CE and the hosted lane; invited capture enables it too.
> Scrubbing happens before chunks, dashboard rows or summary model input are
> built. It covers gitleaks v8.24.3 text patterns (including cloud and GitHub
> keys), Slack `xox*` tokens with 10+ suffix characters, and `sk-` or `tpuf_`
> tokens with 16+ suffix characters. Token suffixes use letters/digits and
> hyphens (plus underscores for `sk-`/`tpuf_`). It also covers PEM private keys
> and certificates, three-part `eyJ…` JWTs, URL user/password credentials,
> `Authorization:` values, and `KEY=value` assignments with 20+ characters from
> letters/digits and `_+/=.-`, and Shannon entropy of at least 3.5. Provider
> rules retain keyword and entropy thresholds; transcript paths, example
> allowlists and `gitleaks:allow` comments do not exempt matches. Overlapping
> matches are replaced and counted once.
>
> Replacements are `[REDACTED:<rule>#<fingerprint>]`. The searchable
> fingerprint is HMAC-SHA256 truncated to 64 bits, keyed by a random
> per-install 32-byte `capture.redact_salt` persisted in mode-0600
> `~/.hev/config.toml` (or `HEV_CONFIG`). Keep that salt stable and private:
> changing it changes future fingerprints. Independent installation salts
> produce different fingerprints. `hev s` reports per-rule replacements from
> the last daemon scan, including attempted writes; these counts are not an
> archive inventory.
>
> The first enabled post-upgrade scan of each Claude/Codex source root deletes
> its old session chunks, blocks, prompts and summaries, verifies deletion, and
> rebuilds all tiers from scrubbed source transcripts, even when signatures are
> unchanged. Cleanup retries after interruption; missing sources can mean loss
> of historical sessions, so `hev index` refuses to remove them unless you pass
> `--allow-source-loss`, and ambiguous ownership stops the upgrade. Stop older
> writers before upgrading. See [archive upgrade
> handling](docs/archive-redaction.md) for scope, failures and restoring raw
> backups.
>
> `redact = false` in `[capture]` explicitly opts out of scrubbing and upgrade
> cleanup; re-enabling scrubbing triggers cleanup again. **Source transcripts
> remain raw.** Arbitrary passwords, customer data, encoded or split secrets,
> and unsupported formats are not guaranteed coverage. Treat the archive,
> source files, config salt and access keys as sensitive even with scrubbing
> enabled.

## Invited?

Run `hev join <code>` (or `hev redeem <code>`) on each Mac you code on. kit
sets up your hosted archive, capture daemon, redaction, and Claude Code/Codex
search skills without Docker or a key to manage. If you already use `hev up`,
your local archive stays in its Docker volume and local capture stops.

Search with `hev traces search "why did the preflight fail"`, browse with `hev open`,
and check machines used at your last join and expiry with `hev s`.
`hev leave` revokes **all machines' invite keys** and deletes the hosted archive;
it stops capture on this Mac and clears its hosted target. A preserved local
target is restored, stopped; `hev up` starts it again. Keys on your other
Macs stop working too; `hev stop` stops their capture jobs.

## Quick start

You need macOS and Docker running.

```bash
brew install hev/tap/kit
hev up
hev traces search "why did the preflight fail"
```

`hev up` starts the hev layer gateway, its Postgres store, the CPU embedding
sidecar and the kit dashboard in Docker, writes `~/.hev/config.toml`, installs
the capture daemon (`hevd`) under launchd, and installs the
[agent skills](#agent-skills) for Claude Code and Codex. The dashboard is at
http://127.0.0.1:8099. The gateway embeds each chunk with
`BAAI/bge-small-en-v1.5` on your CPU, and `hev traces search` gets hybrid search,
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
  ▐█ ▀  ▀ █▌    search     hev traces search "why did the preflight fail"
   ▀██▄▄██▀ ψ   archive    postgres, on this machine · hev-traces
    ▐█  █▌      edition    hev layer community · pro: hev pro
                stop       hev down
```

To build from source, run `go install github.com/hev/kit/cmd/hev@latest`.

## Windows capture

On Windows 11 AMD64, download `hev.exe` or build it with
`go build -o hev.exe ./cmd/hev`, and keep the binary at a stable absolute path.
Configure an existing Layer endpoint and namespace in
`%USERPROFILE%\.hev\config.toml` (or use `hev init`):

```toml
[layer]
endpoint = "https://your-layer-endpoint"
api_key = "your-layer-key"
namespace = "hev-traces"
```

`LAYER_API_KEY` can supply the key instead. For scheduled capture, set it as a
persistent user environment variable before installing; a variable set only in
the current PowerShell session will not reach Task Scheduler. The config file
is the simplest way to provide the scheduled daemon's credentials.

```powershell
.\hev.exe daemon install
.\hev.exe s
```

Installation registers the per-user `hev\hevd` Task Scheduler task with startup
and logon triggers, S4U logon (no stored password), and a one-minute failure
restart interval. Both triggers repeat every minute without an end date,
so retry continues after the scheduler's 999 automatic restarts; running
instances are never overlapped. Registering a startup
trigger may require an elevated terminal for the same user. The task runs from
`%USERPROFILE%` and indexes `.claude\projects` and `.codex\sessions` there.
Logs are in `.hev\hevd.out`, `.hev\hevd.err`, and `.hev\hev.log`.
`hev daemon stop` disables the task and requests a graceful stop;
`hev daemon install` enables and starts it again. `hev daemon uninstall` removes
it. The daemon needs no S3 configuration or secret store. Windows state and
config files use protected ACLs for the current user, SYSTEM and Administrators.

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

The dashboard defaults to hybrid search. Choose **Exact phrase** to match a
contiguous phrase in transcript or evaluator text; the mode persists in its
URL. The [phrase-search contract](docs/hosted-dashboard.md#explicit-phrase-search)
documents normalization, scan limits and the `mode=phrase` API.

## Commands

```text
hev up                  Start the gateway, store, dashboard and daemon (free, local)
hev up --no-dashboard   Start the gateway, store and daemon only
hev up --store <kind>   Move the archive to pgvector or turbopuffer
hev down                Stop the containers and unload the daemon
hev traces search <query>       Search the archive (alias: find)
hev traces list                  List sessions from the last 24h (--since 5d to widen)
hev traces show <session-id>  Show a session
hev                     Read-only nodes, active orders and peers overview
hev --json              Structured overview (partial failures exit 1)
hev nodes/orders/peers   Existing detail commands
hev browse              Browse traces interactively
hev s                   Daemon status: last run, units indexed, last error
hev index               Index once (--tier all includes tool results)
hev init                Configure a hosted Layer
hev pro                 The gateway's edition, and what hev layer pro adds
hev pro trial           Open the hev layer pro trial signup
hev config show         Print the effective config, secrets redacted
```

Bare `hev` reads the existing coordination backend (`HEV_COORDINATION_BACKEND`,
default `hev-control`) for nodes, orders and runs, and the existing peer transport
(`HEV_PEERS_BACKEND`, default `peers`). It does not configure or start anything.
The coordination backend keeps its existing selected-node configuration and
PlanetScale SQL transport configuration; kit does not resolve credentials.

Nodes, orders, runs and peers each get one concurrent read (no retries) with a five-second deadline and a 1 MiB
output bound. Missing configuration, backend errors, malformed JSON and timeouts
produce an **unknown** section while healthy sections still render. Exit 0 means
all reads succeeded; exit 1 means at least one section or peer host read failed.
`hev --help` is offline and performs no reads. Each section names its detail
command; `hev browse` explicitly opens the trace browser.

Nodes cover only the selected node, with no fleet registry or executor readiness
claim. Orders cover the backend list window (currently up to 100 rows ordered by
ID), omit done/cancelled/closed rows and count those omissions **within that
window**. An empty window does not prove there are no active orders elsewhere.
Only selected metadata is printed, never full briefs or pane contents. Peer titles are sanitized and capped at 100 characters, with cwd, branch and
FOH/BOH role when supplied. Nodes project cores, load, mem_free_pct and live
(shown as `reported-workers` in human output) from the host observation;
missing values stay unknown. This count covers workers reported by that observer
and can be zero while native jobs are running. No readiness or idle claim is inferred. Orders retain status and owner; active_runs
joins running/requested/unknown runs by order_id, projecting id, order_id, status,
node_id and assignment role (implementation/validation). All matching runs in the
window are retained. No match means no inferred phase; absence is window-only.
A failed runs read leaves orders visible with phase unknown and a section error.

Overview JSON has `version: 1` and stable `nodes`, `orders`, `peers` objects.
Each has `state` (`ok` or `unknown`), `source`, `scope`, `detail_command`,
`observed_at`, `items`, `sources`, `closed_count`, and nullable `error` with
`code` and `message`. `observed_at` is the read completion time, **not** a
promise of source freshness. Peer items and source summaries retain backend
`source_at` Unix timestamps and `stale` flags, including empty snapshots.
An unknown section can retain successfully parsed peer observations; it never
means idle. JSON includes the complete bounded projected list; terminal output
shows up to eight items and source summaries per section, with long fields
truncated to 256 characters. Human output uses compact labeled rows and source
freshness summaries, never raw source JSON. The runs source summary records
observed_count and its independent list-window scope.

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
instead of their built-in session history. `hev-traces` fans a question out
into several phrasings, gathers and dedupes the hits, and reads a window of
each matching session. `hev up` installs them into `~/.claude/skills` and
`~/.codex/skills` for each harness installed on the machine, and replaces them
on upgrade. To work on a skill, symlink it from a clone instead: `hev up`
leaves a symlinked skill alone.

```bash
ln -sfn "$PWD/skills/hev-traces" ~/.claude/skills/hev-traces
```

### Instruction files

`hevd` also indexes instruction files by default. Set `instructions = false`
under `[capture]` in `~/.hev/config.toml` (or `HEV_CONFIG`) to disable this source.
It reads `~/.claude/CLAUDE.md`, files under `~/.claude/projects/*/memory/`,
and `CLAUDE.md`/`AGENTS.md` between a session workdir and its nearest Git root.
Repositories come from local Claude and Codex sessions; nested workdir instructions
are included. Memory projects use the session workdir when available, otherwise
the Claude project directory name. Global instructions have an empty project.

Whitespace-delimited `@file` imports resolve relative to the importing file;
absolute and `~/` paths are supported. Resolved paths are deduplicated (including
symlink cycles). Scans stop with an error above eight import levels, 4,096 files,
or 1 MiB per file. Missing imports are skipped; other read errors stop discovery.
The source never edits these files.

Content passes through the existing capture redaction policy before L1 chunking
and embedding. The archive's `-instructions` namespace stores one metadata row
per observed file version: canonical path, project, capturing host, mtime, SHA-256
of source bytes, `valid_from`, `valid_to`, and chunk IDs. Validity uses observation
time, not filesystem mtime. Unchanged content is skipped even after a restart;
mtime-only changes do not create versions. Edits and reverts create new versions,
close the previous version, and retain its searchable chunks. Deleted files retain
history; deletion is not currently recorded as a version. Intermediate edits
between scans cannot be recovered.

`hev traces search --json` includes `path`, `project`, `host`, and `version_id` for
instruction hits. Terminal results show their file provenance. Search includes
historical versions; validity is available in the metadata namespace. Transcript
hits keep their session and turn coordinates for `hev trace`.
