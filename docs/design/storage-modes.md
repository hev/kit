# Storage modes for kit

Status: proposal. The board is built (see [board.md](../board.md#two-storage-modes));
everything else here is a plan, not code.

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
3. `HEVLAYER_API_KEY_OP_REF`, read with `op` through `daemon.LayerKey`;
4. only when the caller opts in (`ConfigKey`), `[layer].api_key` in kit's
   config file.

A missing key selects files and is not an error. A key that is configured but
cannot be read (a failing `op read`, a blank variable, a config file that does
not parse) is an error: kit never falls back to files when the operator asked
for Layer. `Peek` reports the mode without reading a secret, for status lines.
Every slice below should call this function rather than test for a key itself.

## Where each piece stands

Sizes are rough: **S** is a day or less, **M** a few days, **L** a week or more.

| Piece | Today | Filesystem mode would mean | Layer mode | Size |
| --- | --- | --- | --- | --- |
| Board | both modes (this change) | `~/.hev/board`, one JSON file per thread, BM25 search | unchanged | done |
| Trace capture and uploader | Layer only (Docker CE with no key) | an on-disk archive of chunks, blocks and sessions | unchanged | L |
| Find / query / search | Layer only | list/show over the archive; lexical search | unchanged (hybrid) | M, after capture |
| Dashboard | Layer only (`hev serve`, Docker) | `hev serve` reads the file archive | unchanged | M, after capture |
| Outcomes and cost | outcomes in Layer, cost in files | outcomes patched into the file archive | unchanged | S, after capture |
| Skills | files (install); capture in Layer | skill loads recorded with the file archive | unchanged | S |
| Peers | files only | unchanged | stays files (open decision) | none |
| Dispatch | files only | unchanged | stays files; namespace passthrough only | none |
| Nodes | files and SSH | unchanged | stays files; `hev join` is a Layer feature | none |
| Secrets | `op` refs, env, config | unchanged | unchanged | none |

### Trace capture and the uploader

**Today.** `hevd` (`internal/daemon`, `internal/index`) refuses to start
without a Layer endpoint, key and namespace. With no key, `hev up` makes one
true anyway: it starts the Docker Layer CE stack (gateway, Postgres with
pgvector, an embedding sidecar, the dashboard) and writes `api_key = "local"`
into `[layer]`. Each scan cycle reads Claude, Codex, Pi, OpenCode and dot
sources and writes four namespaces derived from the base name (default
`hev-traces`): chunks (embedded server side), `-blocks`, `-sessions`,
`-instructions`, plus `-evals`. Local files are bookkeeping only:
`~/.hev/index-state.json` (unit signatures), `daemon-status.json`,
`daemon.lock`, `hev.log`, `sessions-migration.json`. The S3 uploader, SQLite
ledger and envelope spool are legacy and off the live path.

**Filesystem mode.** `index.Run` already writes through a client with a small
surface (upsert chunks, blocks and session rows; patch session rows; list
sessions). A file archive would implement that surface under
`~/.hev/archive/<namespace>/`: append-only JSONL per day for chunks and blocks,
one JSON document per session (so outcomes can patch it), the same lock and
atomic-replace pattern the board uses. No embeddings are computed. Redaction
runs before the write exactly as today, so the files hold only what Layer
would.

**Layer mode.** Unchanged.

**Size: L.** The client interface has to be cut out of `*layer.Client` (today
callers hold the concrete type), schema reconciliation and session-list
migration have to become no-ops for files, and retention needs a decision
(Layer has none to inherit; files grow until pruned). Disk use is the main new
risk: a busy host produces gigabytes of chunks per month.

### Find, query and search (`hev traces`)

**Today.** `cmd/hev/find.go` builds one Layer client for traces list/show/
search, `browse`, `serve`, `eval`, `outcomes` and `pro`, and fails without a
key. List and show use filters and `rank_by` only. Search is hybrid: per
phrasing an ANN leg (server-side embeddings) and a BM25 leg, fused by
reciprocal rank on the server. `hev traces list --factory-dir` already reads a
local `sessions.jsonl`.

**Filesystem mode.** List and show are straightforward scans of the session
documents and block files. Search becomes lexical, as on the board: BM25 per
phrasing, fused by reciprocal rank, filters applied in process. Chunk volume
is far larger than the board's, so a scan per query will not stay fast; an
inverted index on disk (rebuilt incrementally per day file) is the likely
shape, or SQLite FTS5 if a dependency is acceptable (see open decisions).

**Layer mode.** Unchanged.

**Size: M**, after the archive exists.

### The dashboard

**Today.** `hev serve` needs a Layer client. Its core `Reader` interface in
`internal/serve` is only `ListSessionRows` and `ListBlockRows`, a seam a file
reader can fill; eval joins, prefilters and search are reached by type
asserting `*layer.Client`. `hev up` also runs the dashboard container. The
hosted dashboard is Layer by definition. Factory accounting already reads
files.

**Filesystem mode.** `hev serve` with no key serves the file archive through
the `Reader` seam; the Layer-only panels (evals, semantic search) show
lexical search or say they need a key. No Docker.

**Layer mode.** Unchanged, including the hosted dashboard.

**Size: M**, after the archive exists.

### Outcomes and cost

**Today.** Outcomes (PR, CI, merge, revert observations gathered with `gh`)
are patched onto `-sessions` rows by hevd's sweep; they need filters,
`rank_by` and conditional patches, not embeddings. Cost (`internal/cost`,
`prices.toml`, `subscriptions.toml`, `sessions.jsonl`, `outcomes.json`) is
files already and works with no key; per-session cost is also copied onto
Layer session rows.

**Filesystem mode.** The sweep patches the session documents in the file
archive under the archive lock. Cost is unchanged.

**Layer mode.** Unchanged.

**Size: S**, after the archive exists.

### Skills

**Today.** `hev skills setup|sync|check` and skill sources are files only
(`~/.claude/skills`, `~/.codex/skills`, `~/.agents/skills`, state in
`~/.local/state/hev-skills`). Skill capture (`skill_loads` on blocks, `skills`
on sessions) rides on the trace archive.

**Filesystem mode.** Install is unchanged; capture follows the archive.

**Layer mode.** Unchanged.

**Size: S**, part of the capture slice.

### Peers

**Today.** Files only: `~/.local/state/peers-v2` (or `HEV_PEERS_STATE`),
JSONL journals behind the `peers.Store` interface with `platformops.LockPath`.
The optional hevd listener is configured under `[peers]`. Never calls Layer.

**Filesystem mode.** What it is today.

**Layer mode.** Possible gains: a searchable history of peer messages, or
cross-host delivery without SSH. Both are new features rather than a storage
switch, and the journal's guarantees (append-only, lock-held transactions,
exactly-once receipts) are file semantics Layer does not offer directly.

**Size:** none to keep as is; L to move.

### Dispatch

**Today.** Files only: `~/.config/hev/dispatch.toml`, run records under
`~/dispatch/<name>/` (order, meta, result, supervisor state, worktree,
evidence), `~/dispatch/.queue/`. The only Layer touchpoint is a per-host
`namespace` passed to workers as `HEVLAYER_NAMESPACE` so their traces land in
that archive. Expedite posts handoffs with `hev board post`, so it already
follows the board's mode.

**Filesystem mode.** What it is today.

**Layer mode.** Possible gain: searchable order and result history across
hosts. Run records drive live processes and worktrees on one host; they are
operational state, not memory.

**Size:** none to keep as is.

### Nodes

**Today.** `hev nodes` is files plus SSH: `~/.config/hev/nodes/` (invites,
access grants, joined hosts, a lock) and forced-command lines in
`~/.ssh/authorized_keys`. `hev join <code>` is separate and is a Layer
feature: it redeems a code for a hosted endpoint, namespace and key, writes
them to `[layer]`/`[invite]`, and `hev leave` revokes them.

**Filesystem mode.** What it is today. `hev join` stays a way to get a key.

**Layer mode.** Nothing to gain for SSH access state; it must stay local to
the host whose `authorized_keys` it edits.

**Size:** none.

### Secrets

**Today.** Keys come from the environment, from `op://` references resolved by
`op` with a service account (the hevd secrets socket caches them in memory,
never on disk), or as plain values in `config.toml` (mode 0600): the
`[layer]` key, the `[invite]` key, the peers token, bucket credentials and the
redaction salt.

**Filesystem mode / Layer mode.** Secrets are not data kit stores in either
backend, and should not be. The only storage-mode question is the resolver's:
which of these sources count as "a key is configured" (see open decisions).

**Size:** none beyond the resolver.

## Open decisions for Adam

1. **The Docker Layer CE lane in `hev up`.** Today "no key" means `hev up`
   starts Postgres, an embedding sidecar, the gateway and the dashboard in
   Docker. Under the rule, "no key" means files. Options:
   - keep it as an explicit opt-in (`hev up --store pgvector`, recorded in
     `[layer].store`), for people who want semantic search locally without a
     hosted account; the `"local"` placeholder key it writes would then
     count as "a key" for every piece;
   - retire it once the file archive and lexical search land, and drop the
     three images from the release.
   Either way the default `hev up` with no key should stop needing Docker.
   Existing machines on the lane hold their archive in a Docker volume; that
   is the same "do we move data" question as item 4.
2. **Trace search with no embeddings.** Lexical BM25 with phrasings fused by
   reciprocal rank (what the board does) is the floor. Choices above it:
   a pure-Go on-disk inverted index (no new dependency, more code); SQLite
   FTS5 (a real index, but a cgo or large pure-Go dependency); or a small
   local embedding model (heavier, and close to re-creating the CE lane).
   Also: should filesystem-mode search say plainly that it is lexical, so a
   miss is not read as "nothing exists"?
3. **Which file-only pieces gain a Layer mode.** Proposal: peers, dispatch,
   nodes and secrets stay files in both modes; they hold operational state
   tied to one host's processes, worktrees and SSH config. Trace capture,
   search, the dashboard, outcomes and the board are the pieces whose data is
   *memory* and benefit from Layer. If peers or dispatch history should be
   searchable, that is better done by indexing it into the trace archive than
   by moving their journals.
4. **Whether data moves when a key is added later.** The board ships with
   "separate data, no migration": adding a key starts an empty Layer board
   and leaves the files. For traces the stakes are higher (months of
   history). Options: never move (simplest, matches the board); an explicit
   `hev migrate --to layer` that uploads the file archive once; or a
   background backfill when hevd first sees a key. And the reverse: does
   removing a key keep Layer data reachable read-only?
5. **Does `[layer].api_key` in config count as a key?** The trace daemon reads
   it; the board's documented contract is that it reads no key from the TOML
   file, so the board ignores it (`ConfigKey` is off for the board). Making
   every piece agree means either the board starts reading the config key, or
   the daemon stops. The CE lane's `"local"` placeholder lives there too.
6. **Retention for file archives.** Layer has none today; a file archive on a
   laptop needs a default (keep N days, cap size) or a prune command.

## Proposed order of follow-up slices

1. **Resolver everywhere it is cheap.** Have `hev s`/status, `hev up` and
   `hevd` report their mode through `storagemode.Resolve(Peek)`. Decide item 5.
   No behaviour change. *S.*
2. **Archive interface.** Cut a `trace.Archive` interface out of
   `*layer.Client` for what `internal/index`, the outcome sweep and
   `internal/serve` actually call; the Layer client is the first
   implementation. Behaviour identical, fixtures unchanged. *M.*
3. **File archive and capture with no key.** hevd starts with no key and
   writes `~/.hev/archive`. Includes skill loads, outcomes patching and a
   retention default (item 6). *L.*
4. **`hev traces` list/show/search on files**, lexical search per item 2. *M.*
5. **`hev serve` on files** through the `Reader` seam; Layer-only panels say
   so. *M.*
6. **`hev up` with no key stops starting Docker**, and the CE lane becomes
   opt-in or is retired per item 1. *S–M.*
7. **Moving data between modes**, only if item 4 says so. *M.*

Peers, dispatch, nodes and secrets have no slice unless item 3 is decided
differently.
