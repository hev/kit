# RFC 0006: `hev up` — one command from install to backfilling traces

Status: implemented (`hev up`, `hev down`; LYR-89), then amended 2026-09-23
(below). The body from "Summary" on is the original, Postgres-lane design and
is kept as the record of it; where the two disagree, the amendment wins.

## Amendment 2026-09-23: Turbopuffer key required, dashboard in Docker

Layer CE dropped its bundled Postgres store: `public/ce/docker-compose.yml`
on `layer-pro` `main` is the gateway alone, and a non-empty
`TURBOPUFFER_API_KEY` is what selects its store. The keyless local lane this
RFC was built on no longer exists upstream, and the lexical, list-nothing
archive it gave was never what a new user should see first. So:

- **`hev up` requires a Turbopuffer key.** It reads `TURBOPUFFER_API_KEY`, or
  the key a previous `up` stored in the config, and stops before touching
  Docker when there is neither. The key is checked through the gateway before
  the config is written, so a typo fails at `up` and not in the daemon log.
  It is stored as `[layer] api_key` (mode 0600) because the gateway takes it
  as its inbound bearer (`deriveFromStore`) as well as its upstream
  credential.
- **The store is Turbopuffer, in the user's own account.** `store =
  "turbopuffer"`, namespace `hev-traces` by default. Embedding, hybrid search
  and ordered scans all work, so `hev ls`, `hev trace` and the dashboard list
  what `hev find` searches. The capability seam is unchanged; the local stack
  simply stops needing the pgvector row.
- **The dashboard runs in Docker.** Compose runs two services, `gateway` and
  `dashboard` (`hevlayer/kit:<version>`, which is `hev serve` reading through
  the gateway on the Compose network). Only the capture daemon stays on the
  host, under launchd, because it reads transcripts off the host. `[local]
  kit_image` / `HEV_LOCAL_KIT_IMAGE` pin the dashboard image; a release binary
  defaults to the image built from its own tag.
- **`hev down` deletes nothing.** The archive is in Turbopuffer, so there is
  no volume, `--keep-data` is gone, and the index state is never reset by
  `down`.
- **A Postgres-era machine is moved, not stranded.** On a config an older
  `up` wrote, `up` retires the launchd read side (`com.hev.serve`), removes
  the orphaned Postgres container, rewrites the store and the
  `hev-traces-local` namespace, and forgets the index state with the daemon
  stopped, so every transcript is indexed again into the new archive.
- **Install is Homebrew.** `brew install hev/tap/kit`; a tag on `hev/kit`
  publishes the darwin tarballs, pushes `hevlayer/kit:<version>`, and bumps
  `Formula/kit.rb` (`.github/workflows/release.yml`).

```console
$ brew install hev/tap/kit
$ export TURBOPUFFER_API_KEY=tpuf_...
$ hev up
  ✓ docker running
  ✓ layer-gateway:edge running on :8080
  ✓ turbopuffer key accepted, archiving to namespace hev-traces
  ✓ hevd installed, first scan started
  ✓ dashboard running http://127.0.0.1:8099
```

## Summary

`hev up` starts everything kit needs and leaves the archive filling:

```console
$ brew install hev/tap/kit
$ hev up
  ✓ docker running
  ✓ layer-gateway:edge healthy on :8080
  ✓ hevd installed, first scan started
  ✓ http://127.0.0.1:8099

$ hev find "why did the preflight fail"
```

`hev down` is its mirror. Neither asks a question, and neither needs an
account, an API key, or a cloud.

## Motivation

Today a new user runs `go install`, then `hev init` and answers three
prompts about a Layer endpoint they do not have yet, then `hev d`. Every
one of those steps assumes a running Layer somewhere else. The quickstart
therefore starts with "get a Turbopuffer key", which is the opposite of
the wedge: kit is meant to be the thing you install to get your agency
back, not the thing you install after buying a vector database.

Layer CE closed that gap. `public/ce/docker-compose.yml` on `layer-pro`
`main` is a ParadeDB server plus the gateway, and a blank
`TURBOPUFFER_API_KEY` selects the local Postgres backend. A fresh machine
can have a working Layer in one `docker compose up --wait`. `hev up` is
kit taking that for granted.

## Decision

`up` owns the whole local stack, in this order, and each step is a
precondition for the next:

1. **Docker preflight.** If the daemon is missing or stopped, print one
   line saying how to start it and exit non-zero. This is the only
   prerequisite `brew install` cannot supply.
2. **Compose up, pinned, and wait for health.** kit vendors its own copy
   of the CE Compose file and pins `GATEWAY_IMAGE`. `docker compose up
   --wait` plus `GET /health`, which returns a `version` field the
   preflight compares against the pin.
3. **Write the config.** `~/.hev/config.toml` gets `endpoint =
   "http://127.0.0.1:8080"` and the local namespace. This is a file
   write, not an env export, because launchd reads the file selected by
   `HEV_CONFIG` at install time and does not inherit an interactive
   shell's exports. The local gateway is unauthenticated, so `api_key`
   is a placeholder.
4. **Install and start the daemon.** `hev daemon install` writes
   `com.hev.hevd` and fires a cycle immediately. Launchd, not a
   foreground `hev d`, because the promise is that traces keep filling
   after the terminal closes.
5. **Start `hev serve` and print its URL.**

Steps 2 and 4 are ordered deliberately. The daemon's first scan must not
begin before the gateway is healthy: a failed cycle is recorded correctly
and retried, but `capture.scan_interval` defaults to five minutes, so the
user would watch nothing happen for five minutes on the one run that
forms their opinion of the tool.

`up` is idempotent. Run twice, it reports what is already running and
changes nothing. `down` stops the containers and unloads the launchd job,
in that order, because `KeepAlive` otherwise restarts what was just
stopped.

### Switching to a hosted target with `hev init`

`hev init` warns when the existing config contains `[local]`, then allows the
interactive update. The warning says that local and archive settings are
preserved, the local stack is not stopped, and `layer.store` should be reviewed
for the hosted target; it names `HEV_CONFIG` for keeping separate configs.
Refusal would prevent the explicit local-to-hosted transition that `init`
provides, whereas `up` must not repoint a hosted archive as a side effect.

Only `layer.endpoint`, `layer.api_key`, and `layer.namespace` values are changed.
All other bytes, including comments, ordering, unknown keys, capture settings,
`[local]`, buckets, `active_bucket`, projects, and unrelated Layer settings,
survive unchanged. Missing target fields are inserted; existing capture settings
are never defaulted or added. A newly created config still gets
`capture.scan_interval = "5m"`. Enter keeps existing prompt values without
reformatting them, and the stored API key is not printed in its prompt.

This preserves `layer.store = "pgvector"` too: `init` does not infer or change
store capabilities, stop containers, or remove local management settings. The
retained `[local]` block still enables the daemon's first-scan health gate.
Use a separate `HEV_CONFIG` when the hosted lane should have independent store
and local-management settings.

Invalid TOML or non-string target fields fail without rewriting. Updates use a
mode-0600 temporary file beside the config and atomic rename, so a failed write
cannot truncate the original. Config symlinks and other non-regular files are
refused; select the actual file with `HEV_CONFIG` instead.

### Local address family (LYR-106)

The gateway endpoint, readiness probe and persisted Layer endpoint use
`127.0.0.1`, matching Compose's IPv4 loopback publish address. Port checks
test that same address: an IPv6-only listener may coexist, while an IPv4
wildcard listener is a conflict. The check first attempts an IPv4 TCP
connection, then a bind: on macOS, a bind alone can succeed beside a
wildcard listener because of address reuse. No alternate port is chosen.

The read-side readiness probe and printed URL also use `127.0.0.1`
(default `http://127.0.0.1:8099`), because `hev serve` binds there by
default. This avoids resolving `localhost` to an unrelated IPv6 listener.
An already loaded read-side job still keeps its existing flags; it must be
reachable on IPv4 loopback for this readiness check to succeed.

Running `up` updates an older local `localhost` endpoint in the config;
subsequent runs leave the equivalent config untouched. Hosted-config refusal
and preservation of unrelated config keys are unchanged.

### The pin

`GATEWAY_IMAGE` defaults to `hevlayer/layer-gateway:edge` and is
overridable in config. Layer CE tags currently stop at `v0.5.2`, so
pinning to a release does not yet mean anything; `edge` is the honest
pin until 0.6 ships, and moving to `v0.6.x` is a one-value change rather
than a code change.

### The local lane is BM25

Phase one of the Postgres backend serves a pre-embedded dataset: there is
no embed-on-write and no query-time embedding. kit computes no vectors
and will not start, so on this lane retrieval is lexical. Three concrete
changes follow, each forced by the backend rather than chosen:

- **Drop `embed` from the schema.** A field declaration accepts exactly
  `type`, `full_text_search` and `filterable`; `embed` fails every write
  with `UnsupportedByStore: pgvector embed`.
- **Drop `full_text_search` from `workdir`.** Exactly one full-text field
  is allowed. `text` keeps it; `workdir` stays a filterable string.
- **Move off the multi-query passthrough onto `HybridText`.** kit posts
  `{"queries": [...], "rerank_by": ["RRF"]}` today, and that route
  returns `UnsupportedByStore: pgvector multi_query`. The gateway's
  `HybridText` route issues one ranked query per leg and fuses
  gateway-side, and it *is* supported here.

The last one is the reason this is a route change and not a feature
removal. On `HybridText` kit gets a fused BM25 result now and picks up a
dense leg for free whenever the store gains one — no second client
change, no re-plumbing. Phase one requires `fuzziness: 0` on that route
and rejects a cursor or temporal filter; kit's date bounds are plain
scalar `ts`/`id` comparisons, so that restriction does not reach it.

Going semantic later is a **re-index, not a migration**. A namespace
cannot be re-embedded, but kit's source of truth is the harness session
files on disk: drop the namespace, run `hev index`, and the archive comes
back with vectors. The "you can never add the vector column" constraint
binds hosted archives, not local ones.

## Goals

- One command between `brew install` and a searchable archive.
- No account, key, cloud, or manual file edit on that path.
- Same binary still works against a hosted Layer; the local lane is a
  configuration, not a fork.

## Non-goals

- Semantic retrieval on the local lane. It returns when the store can
  embed; nothing in kit changes when it does.
- Kubernetes, multi-host, or anything about operating Layer. `up` runs a
  local stack for one person on one machine.
- Replacing `hev init`. It remains the way to point kit at a hosted
  Layer.

## Proposed surface

```text
hev up                  Start the local stack, daemon, and read side
hev up --no-serve       Skip the read side
hev down                Stop the containers and unload the daemon
hev down --keep-data    Leave the Postgres volume in place
```

Config gains a `[local]` block holding the image pin, the host port and
the Compose project name, so none of the three is a recompile.

## Friction sent back to Layer

Filed on the `layer` team from this work. None of them blocks `up`; kit
works around all four, and that is the point of reporting them.

| Issue | Gap |
|-------|-----|
| LYR-85 | Postgres advertises `Hybrid` as supported, but the multi-query route 422s |
| LYR-86 | Store capabilities are a docs-site build artifact, unreadable at runtime |
| LYR-87 | pgvector rejects more than one `full_text_search` field |
| LYR-88 | `embed` in a schema declaration is a hard failure on stores that cannot embed |

LYR-86 is the one that would let kit stop hardcoding a per-backend branch
and ask the gateway instead.

## Implementation notes

- **The seam.** `layer.Capabilities` has the shape of the gateway's expected
  runtime read (LYR-86: `declared`, `features[]`, `hybrid_routes[]`,
  `schema_limits`). It is filled from a static table keyed by `[layer] store`
  today; an undeclared runtime answer falls back to that table and is never
  read as "supported". The hosted lane's schema and search body are asserted
  byte-identical to what they were before the seam.
- **More than three gaps.** The Postgres backend also refuses
  `upsert_condition`, array schema types, every ordered scan, `aggregate_by`,
  `patch_rows` and `exclude_attributes`. Conditional writes are omitted where
  unsupported. The blocks and sessions namespaces are read only by ordered
  scan, so where the store has none they are not written, and `hev serve`,
  `hev ls` and `hev trace` have nothing to list on the local lane until the
  store gains the scan. `hev up` says so under the URL it prints. Going from
  search-only to a full read side is, again, a re-index:
  `hev index --force --read-side`.
- **Two launchd jobs.** `up` installs `com.hev.hevd` and `com.hev.serve`;
  `down` unloads both, after the containers. A read-side job that is already
  loaded is left exactly as it is. `HEV_LAUNCHD_LABEL` and
  `HEV_SERVE_LAUNCHD_LABEL` rename them, which is what lets a sandbox run
  beside the real jobs.
- **A hosted host is left alone.** `up` checks for a hosted `[layer] endpoint`
  before it starts anything — no pull, no container, no file, no launchd call.
  `down` on a config without a `[local]` block touches neither launchd nor the
  index state (*as implemented, pending a decision on what a managed `down`
  does with a read-side job it did not install*). The index state is reset
  only when the config in effect, env included, is the local stack. Without
  Docker, a managed `down` still unloads its jobs, then fails naming the
  containers it left.
- **The gate is in two places.** `up` does not install the daemon until
  `/health` answers, and a daemon whose config has a `[local]` block holds its
  first scan for the gateway (two minutes at most), which covers login, when
  launchd starts the job before Docker has started the containers.
- **The blank key is forced.** Compose is run with `TURBOPUFFER_API_KEY=`
  whatever the shell exports, because a non-blank key selects the hosted
  backend.

## Open questions

- **Distribution.** `brew install hev/tap/kit` needs a tagged release and
  a generated formula in `hev/homebrew-tap`; kit has never cut a release.
  `hev/factory` has the pattern already (`bump-tap.sh` writing a formula
  on every tag). Tracked separately from this RFC.
- **Namespace naming.** Local and hosted archives cannot be the same
  namespace, since one is lexical and one is embedded. Whether `up`
  should suffix the local one, or refuse to reuse a hosted name, is
  undecided. *As implemented, pending a decision:* the local namespace
  defaults to `hev-traces-local`, and `up` refuses to touch a config whose
  `[layer] endpoint` is not on this machine, naming `HEV_CONFIG` as the way
  to keep both.
- **Port collisions.** 8080 is a popular port. Whether `up` should detect
  and offer the next free one, or simply fail with the override to set,
  is undecided. *As implemented, pending a decision:* fail, naming
  `HEV_LOCAL_PORT` / `[local] port` (and `HEV_LOCAL_SERVE_PORT` /
  `[local] serve_port` for the read side). `up` never picks a port itself.
