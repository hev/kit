# Storage modes for kit

Status: built for the board and for traces. The board shipped first (see
[board.md](../board.md#two-storage-modes)). Trace capture, find, query and
search, the dashboard, outcomes and per-session cost, and skill capture now
have a filesystem mode too: a file archive with lexical search. The decisions
behind it are recorded [below](#decisions).

## The rule

**No Layer API key means kit stores its data in files on the host it runs on.
A Layer API key means kit writes to Layer.** The mission already asks for
local operation to stay useful without a hosted account; this makes "useful"
mean "works with no key and no Docker".

One function decides the mode: `storagemode.Resolve` in
`internal/storagemode`. It reads the key lookup kit already has:

1. variables the caller names first (the board passes
   `FACTORY_BOARD_LAYER_API_KEY`);
2. `HEVLAYER_API_KEY` and its deprecated aliases;
3. `HEVLAYER_API_KEY_OP_REF`, read with `op` through `layerenv.ReadKey`;
4. only when the caller opts in (`ConfigKey`), `[layer].api_key` in kit's
   config file. The trace daemon opts in; the board does not.

A missing key selects files and is not an error. The placeholder `local` is not
a key wherever it appears. A key that is configured but cannot be read (a
failing `op read`, a blank variable, a config file that does not parse) is an
error: kit never falls back to files when the operator asked for Layer. `Peek`
reports the mode without reading a secret, for status lines. Every piece with two
modes calls this function rather than test for a key itself.

## Where each piece stands

| Piece | Filesystem mode | Layer mode |
| --- | --- | --- |
| Board | `~/.hev/board`, one JSON file per thread, BM25 search | unchanged |
| Trace capture | file archive under `~/.hev/archive/<namespace>/` | unchanged |
| Find / query / search | list/show over the archive; lexical search | unchanged (hybrid) |
| Dashboard | `hev serve` reads the file archive, no container | unchanged |
| Outcomes and cost | outcomes patched into the file archive; cost in files | unchanged |
| Skills | install in files; skill loads recorded with the archive | unchanged |
| Peers | files only | stays files |
| Dispatch | files only | stays files; namespace passthrough only |
| Nodes | files and SSH | stays files; `hev join` is a Layer feature |
| Secrets | `op` refs, env, config | unchanged |

### Trace capture

With no key, `hev up` writes the config, installs hevd under launchd or
systemd, installs the skills, and starts nothing in Docker. hevd writes
through `index.Archive`, the storage surface cut out of `*layer.Client`; the
Layer client is one implementation and `internal/filearchive` is the other.

The file archive lives at `~/.hev/archive/<namespace>/` (namespace default
`hev-traces`). `[archive] dir` in the config or `HEV_ARCHIVE_DIR` moves the
root. It holds `sessions/<shard>/<id>.json`, `chunks/<shard>/<id>.jsonl`,
`blocks/<shard>/<id>.jsonl`, `instructions.jsonl` and `index/`. Every write
takes a lock file and replaces files atomically, as the board does, on Linux,
macOS and Windows. Redaction runs before the write exactly as for Layer, so the
files hold only what Layer would. No embeddings are computed. See
[the trace archive](../trace-archive.md).

### Find, query and search (`hev traces`)

`hev traces list|show|search`, `hev browse`, `hev outcomes` and `hev index`
work on the file archive. List and show scan session documents and block
files. Search is lexical: BM25 per phrasing over the on-disk index, with
`--also` phrasings fused by reciprocal rank. `--plan`, `--workdir`,
`--harness`, `--since` and `--order` still work. The command says the search
matches words, not meaning, so a miss is not read as "nothing exists".

### The dashboard

`hev serve` with no key serves the file archive through the `Reader` seam in
`internal/serve`, with no container. Search there is lexical ("Words" mode)
plus exact phrase. Grades, evals and semantic search need a Layer API key, and
the page says so. The hosted dashboard is Layer by definition.

### Outcomes and cost

hevd's sweep patches outcomes (PR, CI, merge, revert) onto the session
documents in the file archive under the archive lock. Cost (`internal/cost`)
was files already and works with no key; per-session cost is recorded on the
session in either mode.

### Skills

`hev skills setup|sync|check` and skill sources are files only. Skill capture
(`skill_loads` on blocks, `skills` on sessions) follows the trace archive.

### Peers

**Today.** Files only: `~/.local/state/peers-v2` (or `HEV_PEERS_STATE`),
JSONL journals behind the `peers.Store` interface with `platformops.LockPath`.
The optional hevd listener is configured under `[peers]`. Never calls Layer.

**Filesystem mode.** What it is today.

**Layer mode.** Stays files (decision 3). Possible gains: a searchable
history of peer messages, or cross-host delivery without SSH. Both are new features rather than a storage
switch, and the journal's guarantees (append-only, lock-held transactions,
exactly-once receipts) are file semantics Layer does not offer directly.

### Dispatch

**Today.** Files only: `~/.config/hev/dispatch.toml`, run records under
`~/dispatch/<name>/` (order, meta, result, supervisor state, worktree,
evidence), `~/dispatch/.queue/`. The only Layer touchpoint is a per-host
`namespace` passed to workers as `HEVLAYER_NAMESPACE` so their traces land in
that archive. Expedite posts handoffs with `hev board post`, so it already
follows the board's mode.

**Filesystem mode.** What it is today.

**Layer mode.** Stays files (decision 3). Possible gain: searchable order
and result history across hosts. Run records drive live processes and worktrees on one host; they are
operational state, not memory.

### Nodes

**Today.** `hev nodes` is files plus SSH: `~/.config/hev/nodes/` (invites,
access grants, joined hosts, a lock) and forced-command lines in
`~/.ssh/authorized_keys`. `hev join <code>` is separate and is a Layer
feature: it redeems a code for a hosted endpoint, namespace and key, writes
them to `[layer]`/`[invite]`, and `hev leave` revokes them.

**Filesystem mode.** What it is today. `hev join` stays a way to get a key.

**Layer mode.** Nothing to gain for SSH access state; it must stay local to
the host whose `authorized_keys` it edits.

### Secrets

**Today.** Keys come from the environment, from `op://` references resolved by
`op` with a service account (the hevd secrets socket caches them in memory,
never on disk), or as plain values in `config.toml` (mode 0600): the
`[layer]` key, the `[invite]` key, the peers token, bucket credentials and the
redaction salt.

**Filesystem mode / Layer mode.** Secrets are not data kit stores in either
backend, and should not be. The only storage-mode question is the resolver's:
which of these sources count as "a key is configured" (decision 5).

## Decisions

1. **The keyless Docker lane is retired, not kept as an opt-in.** `hev up`
   with no key starts nothing in Docker, and its `--store` choice is gone. A
   host whose config still names the old store is told that traces now go to
   files; hevd re-reads every transcript still on disk into the file archive.
   kit never stops, starts or deletes the old containers or volume; the
   operator runs `docker compose -p hev-kit down` and removes the volume with
   `docker volume rm` only if sure. The Turbopuffer lane
   (`hev up --store turbopuffer`) is unchanged.
2. **Trace search with no key is a pure-Go on-disk BM25 index.** Immutable
   segments plus a manifest under `index/`. No embeddings, no SQLite. Search
   says it is lexical.
3. **Peers, dispatch, nodes and secrets stay files in both modes.** They hold
   operational state tied to one host's processes, worktrees and SSH config.
   If their history should be searchable, index it into the trace archive
   rather than move their journals.
4. **Data does not move between modes.** Adding a key later starts a Layer
   archive and leaves the files. `hev backfill` from the original session
   files brings old sessions to Layer. The board works the same way.
5. **For traces, `[layer].api_key` counts as a key** and the placeholder
   `local` counts as none. The board still ignores the config key.
6. **Retention: no pruning by default.** Files grow until removed. Disk use is
   documented in [the trace archive](../trace-archive.md).
