# The trace archive with no Layer API key

With no Layer API key, kit keeps traces in files on the host it runs on. No
Docker, no endpoint and no account are needed. With a key, traces go to hev
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

`~/.hev/archive/<namespace>/`, namespace `hev-traces` by default
(`HEVLAYER_NAMESPACE` or `[layer].namespace` change it). Move the root with

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
  instructions.jsonl           instruction file versions (when captured)
  index/                       the lexical search index
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
`hev serve` — can share the directory on Linux, macOS and Windows. Concurrent
capture and search are covered by a test with parallel writers and readers in
goroutines and in separate processes.

## Search

There are no embeddings. Search is lexical: Okapi BM25 per phrasing over chunk
text (and the tool name), and `--also` phrasings ranked on their own and fused
by reciprocal rank, the way extra phrasings widen a Layer search. It matches
the words in a phrasing, not their meaning, and `hev traces search` says so;
add `--also` phrasings with the exact words you expect (an error string, a
flag, a file name).

`--order`, `--plan`, `--workdir`, `--harness` and `--since` work as they do on
Layer. `hev serve` offers "Words" and exact-phrase search over the same index.
Grades (eval rows) and semantic search need a key; the dashboard says so.

The index is a pure-Go inverted index in `index/`: immutable segment files
(sorted term dictionary, postings, a document table) and a manifest naming
which segment holds each session's current postings. A capture only marks the
session; hevd folds the marks into a new segment after each scan, and any
search does it first if hevd has not. Segments are merged, dropping the
postings of rewritten and deleted sessions, when there are more than eight or
one is mostly dead. Deleting `index/` is safe: the next search or scan
rebuilds it from the chunk files.

## Disk use

Measured on a real host: every Claude Code and Codex transcript on the machine
(2,503 transcript files, 1.86 GB), indexed with the default tiers (text and
tool use) into a fresh archive.

| | on disk | per 1,000 sessions |
| --- | ---: | ---: |
| sessions (2,431 rows) | 46 MB | 19 MB |
| chunks (216,111 rows) | 321 MB | 132 MB |
| blocks (169,402 rows) | 556 MB | 229 MB |
| search index | 49 MB | 20 MB |
| **total** | **970 MB** | **~400 MB** |

The archive is about half the size of the transcripts it was read from. Blocks
are the largest part because they keep tool results whole for the transcript
view; chunks keep only the searchable tiers. The first capture of that host
took 4 m 43 s and 300 MB of memory.

Nothing is pruned by default: the archive grows with every session until you
remove it. To start over, stop hevd (`hev down`) and delete the namespace
directory: a new archive has a new identity, so the next scan (or
`hev index`) reads every transcript still on disk into it.

## Search speed

Measured by `TestScaleSearch` (`internal/filearchive/scale_test.go`, opt-in):
the archive above copied ten times under new session ids, 24,310 sessions and
2,161,110 chunks (4.0 GB), written through the ordinary write path, on one
Linux host with an SSD. Writing the copies took about 2 minutes and building
the index from nothing 52 s (5 segments, 470 MB). Times are for the store
call, warm page cache; each was run twice and both runs are within the ranges
below.

| | time |
| --- | ---: |
| list every session | 0.5 s |
| list the last 7 days (top 1,000) | 0.4 s |
| search, one word | 0.1 s |
| search, a question | 0.3 s |
| search, three phrasings (`--also`) | 0.8–1.0 s |
| search, `--since 7d` | 0.1 s |
| search, `--harness codex` | 0.4–0.5 s |
| dashboard search (200 candidates) | 0.7–1.0 s |

On the real archive of 2,489 sessions, `hev traces search` returns in about
0.2 s and `hev traces list` in 0.2 s, process start included. A run on the
same host under heavy load from other work took up to 2.5 s for the
three-phrasing search. The whole measurement run (writing the copies and
building the index in one go) peaked at about 1.9 GB of memory. hevd folds
each scan into the index as it goes, so only a first scan of a large history,
or a deleted index, builds it all at once; the first capture of the real
archive above peaked at 300 MB.

## Moving between modes

Data does not move on its own. Adding a key later starts a Layer archive; send
the sessions still on disk there with `hev backfill`. Removing a key starts
filling the file archive; hevd re-reads every transcript still on disk into
it. The file archive and a Layer archive are never merged.

## The retired local Docker lane

Older kit releases started a local gateway, store and embedding sidecar in
Docker when there was no key. That lane is retired. A host whose config still
names its store gets a message saying so, how to bring old sessions across,
and how to stop and remove the old containers and volume yourself; kit never
starts, stops or deletes them, and `hev up` moves the config to files.
