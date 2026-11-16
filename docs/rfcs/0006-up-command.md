# RFC 0006: `hev up` — one command from install to backfilling traces

Status: implemented (`hev up`, `hev down`; LYR-89), amended 2026-09-23,
2026-09-27, three times on 2026-09-28, 2026-11-05 and 2026-11-10 (all below).
The body from "Summary" on is the original design, trimmed on 2026-11-10 to
the parts that still hold; where they disagree, the latest amendment wins.

## Update 2026-11-10: the keyless Docker lane is retired

The keyless local lane this RFC was built around is retired. It ran a local
store, the gateway and the dashboard in Docker. Now a host with no hev layer
key keeps its traces in files: `hev up` writes the config, installs the
capture daemon and the skills, and starts nothing in Docker. Capture writes to a file archive under `~/.hev/archive/`, and search
over it is lexical. See [the trace archive](../trace-archive.md) and
[storage modes](../design/storage-modes.md).

The old `--store` choice for that lane is gone. On a host whose config still
names it, `hev up` says traces now go to files and that hevd re-reads every
transcript still on disk into the file archive. To keep traces in Layer
instead, set `HEVLAYER_API_KEY` and use `hev backfill`. kit never stops,
starts or deletes the old containers or volume; the operator stops them with
`docker compose -p hev-kit down`. A key keeps everything else here: hosted
hev layer, and `hev up --store turbopuffer` with a local gateway and dashboard
in front of the operator's own Turbopuffer account.

## Amendment 2026-11-05: kit feeds hosted hev layer

The current environment contract and lane selection are specified in
[Capture and search with hev layer](../hevlayer.md). `HEVLAYER_API_KEY` selects
the public hosted gateway and hosted dashboard sign-in on a fresh machine,
without starting a local gateway. Saved targets persist unless a store choice
explicitly moves them. Saved keys win over exported keys during `hev up`. With no key,
capture writes to the file archive (2026-11-10 update above). Provider
accounts require an explicit advanced store choice. Deprecated aliases are
nonfatal fallbacks, and canonical names take precedence. No new transport
is needed: the gateway speaks the existing client API.

The dated amendments below describe historical decisions. Their automatic
provider-account selection is superseded by this amendment.

## Amendment 2026-09-28 (third): Layer 0.7.3, telemetry names kit

LYR-141 (kit v0.3.3). Layer 0.7.3 reads two new gateway variables and sends
them as `distribution` and `distributionVersion` in its anonymous
`gateway_started` and `gateway_heartbeat` events. So:

- **The pin moves.** `[local] image` defaults to
  `hevlayer/layer-gateway:0.7.3`; the 0.7.2 default joins the recorded ones
  that follow an upgrade.
- **The gateway says kit started it.** The vendored Compose file sets
  `LAYER_TELEMETRY_SOURCE=kit` and `LAYER_TELEMETRY_SOURCE_VERSION` from
  `KIT_VERSION`, which `hev up` sets, never inherits, to the version the
  release build stamps into the binary (`dev` outside one). Nothing else
  about the user or the archive is added.
- **The opt-outs are unchanged.** Telemetry stays on by default;
  `DO_NOT_TRACK=1` or `HEVLAYER_TELEMETRY=off` in the shell that runs `hev up`
  still turns every event off.

## Amendments 2026-09-27 to 2026-09-28 (second): the keyless lane

*Retired by the 2026-11-10 update above.*

These three amendments made a key optional again and specified the keyless
Docker lane: its store, its gateway pins (0.7.0 to 0.7.2) and what that
store could and could not serve. All of it is gone with
the lane. Two decisions from them still hold on every Layer lane:

- **A rescan keeps a summary.** An upsert replaces the whole row, so hevd
  rescanning a transcript that grew used to write the session row back with
  an empty summary. A session write now first reads the stored `summary`
  (that attribute alone) for every row parsed without one, and carries it
  into the upsert. Only a non-empty summary, a harness title or
  `--summarize`, replaces a stored one. It costs one small read per session
  batch and no second write.
- **Model per store.** Turbopuffer uses `qwen/qwen3-embedding-8b`.
  `HEVLAYER_EMBED_MODEL` overrides it.

## Advanced historical BYO amendment 2026-09-23: provider key and Docker dashboard

*Superseded in part by the 2026-09-27 amendment above: the key became
optional again. The keyless lane that amendment added was retired on
2026-11-10.*

Layer CE dropped its bundled keyless store: `public/ce/docker-compose.yml`
on `layer-pro` `main` is the gateway alone, and a non-empty
the gateway's provider key selects its store. The keyless local lane this
RFC was built on no longer exists upstream, and the lexical, list-nothing
archive it gave was never what a new user should see first. So:

- **`hev up` requires a Turbopuffer key.** It reads `HEVLAYER_API_KEY`, or
  the key a previous `up` stored in the config, and stops before touching
  Docker when there is neither. The key is checked through the gateway before
  the config is written, so a typo fails at `up` and not in the daemon log.
  It is stored as `[layer] api_key` (mode 0600) because the gateway takes it
  as its inbound bearer (`deriveFromStore`) as well as its upstream
  credential.
- **The store is Turbopuffer, in the user's own account.** `store =
  "turbopuffer"`, namespace `hev-traces` by default. Embedding, hybrid search
  and ordered scans all work, so `hev traces list`, `hev traces show` and the dashboard list
  what `hev traces search` searches. The capability seam is unchanged.
- **The dashboard runs in Docker.** Compose runs two services, `gateway` and
  `dashboard` (`hevlayer/kit:<version>`, which is `hev serve` reading through
  the gateway on the Compose network). Only the capture daemon stays on the
  host, under launchd, because it reads transcripts off the host. `[local]
  kit_image` / `HEV_LOCAL_KIT_IMAGE` pin the dashboard image; a release binary
  defaults to the image built from its own tag.
- **`hev down` deletes nothing.** The archive is in Turbopuffer, so there is
  no volume, `--keep-data` is gone, and the index state is never reset by
  `down`.
- **An older keyless machine is moved, not stranded.** On a config an older
  `up` wrote, `up` retires the launchd read side (`com.hev.serve`), removes
  the orphaned store container, rewrites the store and the
  `hev-traces-local` namespace, and forgets the index state with the daemon
  stopped, so every transcript is indexed again into the new archive.
- **Install is Homebrew.** `brew install hev/tap/kit`; a tag on `hev/kit`
  publishes the darwin tarballs, pushes `hevlayer/kit:<version>`, and bumps
  `Formula/kit.rb` (`.github/workflows/release.yml`).

```console
$ brew install hev/tap/kit
$ export HEVLAYER_API_KEY=tpuf_...
$ hev up --store turbopuffer
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

$ hev traces search "why did the preflight fail"
```

`hev down` is its mirror. Neither asks a question. (As first designed, this
ran a keyless local store in Docker; since 2026-11-10 the keyless path starts
nothing in Docker and the Docker stack belongs to the Turbopuffer lane.)

## Motivation

Today a new user runs `go install`, then `hev init` and answers three
prompts about a Layer endpoint they do not have yet, then `hev d`. Every
one of those steps assumes a running Layer somewhere else. The quickstart
therefore starts with "get a Turbopuffer key", which is the opposite of
the wedge: kit is meant to be the thing you install to get your agency
back, not the thing you install after buying a vector database.

Layer CE closed that gap at the time: its Compose file ran a local store
next to the gateway, and a blank provider key selected it. A fresh machine
could have a working Layer in one `docker compose up --wait`. `hev up` was
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
   "http://127.0.0.1:8080"` and the namespace. This is a file
   write, not an env export, because launchd reads the file selected by
   `HEV_CONFIG` at install time and does not inherit an interactive
   shell's exports.
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

This preserves `layer.store` too: `init` does not infer or change
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

## Goals

- One command between `brew install` and a searchable archive.
- No manual file edit on that path.
- Same binary still works against a hosted Layer; the local lane is a
  configuration, not a fork.

## Non-goals

- Kubernetes, multi-host, or anything about operating Layer. `up` runs a
  local stack for one person on one machine.
- Replacing `hev init`. It remains the way to point kit at a hosted
  Layer.

## Proposed surface

```text
hev up                  Start the local stack, daemon, and read side
hev up --no-serve       Skip the read side
hev down                Stop the containers and unload the daemon
```

Config gains a `[local]` block holding the image pin, the host port and
the Compose project name, so none of the three is a recompile.

## Friction sent back to Layer

Filed on the `layer` team from this work. None of them blocks `up`; kit
works around both, and that is the point of reporting them. (Two more,
LYR-85 and LYR-87, concerned only the retired keyless store.)

| Issue | Gap |
|-------|-----|
| LYR-86 | Store capabilities are a docs-site build artifact, unreadable at runtime |
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

## Open questions

- **Distribution.** `brew install hev/tap/kit` needs a tagged release and
  a generated formula in `hev/homebrew-tap`; kit has never cut a release.
  The release tooling has the pattern already (`bump-tap.sh` writing a formula
  on every tag). Tracked separately from this RFC.
- **Namespace naming.** Closed by the 2026-11-10 update: the keyless lane
  that needed its own namespace is retired.
- **Port collisions.** 8080 is a popular port. Whether `up` should detect
  and offer the next free one, or simply fail with the override to set,
  is undecided. *As implemented, pending a decision:* fail, naming
  `HEV_LOCAL_PORT` / `[local] port` (and `HEV_LOCAL_SERVE_PORT` /
  `[local] serve_port` for the read side). `up` never picks a port itself.
