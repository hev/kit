# The trace archive with no Layer API key

With no Layer API key (Starter Kit), kit keeps traces in files on the host it
runs on and searches them with grep. No Docker, no database, no embeddings,
no endpoint and no account are needed. With a key, traces go to hev
layer as before and nothing is written here. The rule and how it is decided
are in [storage modes](design/storage-modes.md).

## What decides the mode

`storagemode.Resolve` looks for a key in this order: `HEVLAYER_API_KEY` (and
its deprecated aliases), `HEVLAYER_API_KEY_OP_REF` read with `op`, then
`[layer].api_key` in `~/.hev/config.toml`. A key saved in the config counts,
because that is where hosted `hev up` saves it. The placeholder `local`, which
the retired local Docker lane wrote, does not count. A key that is configured
but cannot be read (a failing `op read`, a blank variable, a config file that
does not parse) is an error: kit never falls back to files when you asked for
Layer.

`hev daemon status` prints which mode the host is in, without reading a
secret.

## Where it lives

`~/.hev/traces/<namespace>/`, namespace `hev-traces` by default
(`HEVLAYER_NAMESPACE` or `[layer].namespace` change it).

A machine that already has `~/.hev/archive/` (every install before the
unified `~/.hev/<surface>/` layout) and no `~/.hev/traces/` keeps using
`~/.hev/archive/`: nothing is moved or re-indexed. The rule, the same for
traces, dispatch run records and peers state:

1. `HEV_ARCHIVE_DIR`, then `[archive] dir`, win over everything;
2. otherwise `~/.hev/traces/` when it exists;
3. otherwise `~/.hev/archive/` when it exists;
4. otherwise `~/.hev/traces/` (a fresh install; `hev up` creates it).

`hev up` never creates `~/.hev/traces/` beside a kept `~/.hev/archive/`,
since the new directory would win as soon as it existed. To move a legacy
archive, stop hevd, `mv ~/.hev/archive ~/.hev/traces`, and start it again.

Move the root with

```toml
[archive]
dir = "/data/hev-archive"
```

or `HEV_ARCHIVE_DIR`, which wins over the config.

```text
<namespace>/
  .lock                        held by every read and write
  meta.json                    the archive's identity and redaction baseline
  sessions/<shard>/<id>.json   one session row
  chunks/<shard>/<id>.jsonl    a session's searchable chunks
  blocks/<shard>/<id>.jsonl    a session's whole blocks, for the transcript view
  index.jsonl                  one line per session, for listing newest first
  evals.jsonl                  evaluations from `hev eval put`
  instructions.jsonl           instruction file versions (when captured)
  blobs/sha256/<2 hex>/<hash>  image bytes from transcripts (created on the first image)
```

`blobs/` holds the images transcripts carried, by sha256, and the block rows
carry only their metadata and a marker in the text. It is used in Layer mode
too, since Layer has no object store: the bytes stay on the capturing host.
See [images in traces](trace-images.md), including how to turn it off
(`[capture] images = false`).

The rows are the ones hevd would send Layer, in the same shapes, after the
same redaction. Session rows are patched in place by the outcome sweep and by
summaries, with the same ownership rules as on Layer: a capture never rolls a
newer row back, and never overwrites outcomes or summaries.

Every operation takes `.lock` (the lock the board and peers use) and every
file is replaced atomically, so any number of processes — hevd, `hev traces`,
`hev serve` — can share the directory on Linux, macOS and Windows. A
read-modify-write is conditional: the new file is renamed into place only if
the old one still holds the bytes that were read, so a change made outside
kit is refused rather than overwritten. Evaluations are write-once by id, as
on Layer. `index.jsonl` is derived: when it does not name exactly the session
files on disk (a crash between the two writes, a file removed by hand, an
archive from a 0.7 preview) the next listing rebuilds it.

Redaction delete (the archive redaction migration, see
[archive redaction](archive-redaction.md)) removes the session's chunk, block
and session files, its evaluations and its `index.jsonl` line. Concurrent
capture and search are covered by a test with parallel writers and readers in
goroutines and in separate processes.

## Search

There is no index and nothing is embedded: search is grep over the chunk
files. A phrasing is split into words (lowercased; stopwords such as "why",
"did" and "the" are dropped; simple plurals fold, so `ports` finds `port`
and `retries` finds `retry`). A chunk matches when its text (and tool name)
contains **every** word of a phrasing, as a case-insensitive substring. Each
`--also` phrasing is an alternative: a chunk that matches any of them is a
hit. Hits rank by how many distinct words they contain across all phrasings,
then newest first. `--regex` takes the query as one Go (RE2) regular
expression instead; prefix `(?i)` to ignore case. It matches words, not
meaning, and `hev traces search` says so on stderr; `--json` adds a `snippet`
around the first match.

When `rg` (ripgrep) is on PATH, kit runs `rg --json` (fixed strings,
case-insensitive) to find the lines that hold any of the words, then decides
each one in Go on the decoded text, so a word that only appears in a field
name never matches. Without `rg`, kit reads the files itself; the results are
the same, only slower. Words that JSON escapes (quotes, backslashes, `<`, `>`,
`&`) and regular expressions always take the Go path. `HEV_GREP=go` forces it.

`--order`, `--plan`, `--workdir`, `--harness`, `--since` and `--context` work
as they do on Layer. `hev serve` offers word search and exact-phrase search
over the same files, and grades from `hev eval put`; search by meaning needs a
hev layer key, and the dashboard says so.

The dashboard has two tabs: **Traces** (the session list, its Stats, and a
session's transcript and timeline) and **Board**, which browses and searches
the board (files under `~/.hev/board` with no key; see
[the board](board.md#browse-it-in-hev-serve)). There is no Asks tab: it was
removed from `hev serve` in 0.7.

## Disk use

Measured on a real host: every Claude Code and Codex transcript on the machine
(2,503 transcript files, 1.86 GB), indexed with the default tiers (text and
tool use) into a fresh archive.

| | on disk | per 1,000 sessions |
| --- | ---: | ---: |
| sessions (2,431 rows) | 46 MB | 19 MB |
| chunks (216,111 rows) | 321 MB | 132 MB |
| blocks (169,402 rows) | 556 MB | 229 MB |
| **total** | **~920 MB** | **~380 MB** |

The archive is about half the size of the transcripts it was read from. Blocks
are the largest part because they keep tool results whole for the transcript
view; chunks keep only the searchable tiers. The first capture of that host
took 4 m 43 s and 300 MB of memory. (Measured with the 0.7 preview, which
also kept a 49 MB search index; grep search keeps none.)

Nothing is pruned by default: the archive grows with every session until you
remove it. To start over, stop hevd (`hev down`) and delete the namespace
directory: a new archive has a new identity, so the next scan (or
`hev index`) reads every transcript still on disk into it.

## Search speed

Measured on 2026-11-17 on a Linux host with 32 cores and an SSD, shared with
other work: every Claude Code and Codex transcript on the machine (1,736
transcript files) indexed with the default tiers into a fresh archive, 1,629
sessions and 213,268 chunks (291 MB of chunk files; the archive is 935 MB with
blocks). The first capture took 10 m 30 s and peaked at 390 MB of memory.
Each figure is the best of three runs of the whole command, process start
included, with ripgrep 14 on PATH and with `HEV_GREP=go`.

| | with rg | without rg |
| --- | ---: | ---: |
| `hev traces ls` (the newest sessions) | 0.3 s | 0.3 s |
| search, one word | 0.3 s | 0.8 s |
| search, a question (`why did the deploy time out`) | 0.4 s | 0.8 s |
| search, three phrasings (`--also` ×2) | 1.6 s | 0.9 s |
| search, `--since 7d` | 0.3 s | 0.7 s |
| search, `--harness codex` | 0.2 s | 0.8 s |
| search, `--regex` (always the Go path) | 1.0–1.6 s | 1.0 s |

Search reads every chunk file a query could touch, so its time grows with
the archive: about linearly in the bytes under `chunks/`. With ripgrep only
the lines holding the rarest-looking word of each phrasing are decoded; a
phrasing of common words (`out`, `time`) narrows less. Without ripgrep kit
reads the files on up to 16 goroutines. Listing reads `index.jsonl` and the
session files it needs.

## Moving between modes

Data does not move on its own. Adding a key later starts a Layer archive; send
the sessions still on disk there with `hev backfill`. Removing a key starts
filling the file archive; hevd re-reads every transcript still on disk into
it. The file archive and a Layer archive are never merged.

## The retired local Docker lane

Older kit releases started a local gateway, Postgres and an embedding sidecar
in Docker when there was no key. That lane is gone. On a host whose config
still names it, `hev up` moves it to files:

- if `docker` is here and the lane's containers start, every row (chunks,
  blocks, sessions, evaluations, instruction versions) is exported from the
  old gateway into the file archive, so sessions whose transcripts are gone
  survive; containers `hev up` started are stopped again;
- otherwise the index state is reset so hevd re-reads every transcript still
  on disk, and `hev up` prints how many sessions could not be recovered
  because their transcripts are gone; those stay only in the Docker volume.

kit never removes the containers or the volume. `docker compose -p hev-kit
down` stops them; remove the volume with `docker volume rm` only when you no
longer need it.
