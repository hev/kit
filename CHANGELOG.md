# Changelog

## Unreleased

### Traces with no Layer API key are kept in files

- A host with no Layer API key keeps its traces in a file archive under
  `~/.hev/archive/<namespace>/` (`[archive] dir`, or `HEV_ARCHIVE_DIR`).
  `hevd` starts and captures with no key, no endpoint and no Docker;
  redaction runs before the write as it does for Layer. See
  [the trace archive](docs/trace-archive.md).
- `hev traces list`, `show` and `search`, `hev browse`, `hev outcomes` and
  `hev index` work on the file archive. Search there is lexical (BM25 per
  phrasing, `--also` phrasings fused by reciprocal rank) and says so.
- `hev serve` with no key serves the dashboard from the file archive. Grades
  and semantic search need a key, and the page says so.
- With a key nothing changes: hosted hev layer, and the Turbopuffer lane
  (`hev up --store turbopuffer`), work as before.
- A key saved in `[layer].api_key` counts as a key; the placeholder `local`
  does not. A key that is configured but cannot be read is an error, never a
  switch to files.

### Retired: the local Postgres lane

- `hev up` with no key no longer starts the Layer CE gateway on Postgres with
  pgvector, the CPU embedding sidecar or the dashboard container. It writes
  the config, installs `hevd` and the skills, and starts nothing in Docker.
- `--store pgvector`, the `local` placeholder key, the embedding sidecar image
  and `docker-compose.postgres.yml` are gone, along with the session-list
  migration for archives an early kit wrote on Postgres.
- A config that still names `store = "pgvector"` gets a message saying the
  lane is retired, that its traces now go to files, how to bring old sessions
  across (`hevd` re-reads transcripts still on disk; `hev backfill` sends them
  to Layer once a key is set) and how to stop and remove the old containers
  and volume. kit never starts, stops or deletes them. `hev up` moves such a
  config to files.
