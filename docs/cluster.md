# hevd cluster

A cluster gives one operator's machines a continuously refreshed view of one
another: membership, versions and bounded usage facts. It runs in hevd using
SWIM with Lifeguard (HashiCorp memberlist), without a leader. It is optional;
without `[cluster]`, no cluster TCP, UDP or local IPC listener starts.

```toml
[cluster]
bind = "192.0.2.10:7946"
advertise = "192.0.2.10:7946" # optional; useful behind address translation
seeds = ["192.0.2.11:7946"]
key_ref = "secret://hev-cluster-key"
name = "example" # optional; defaults to hostname; stable across restarts
# allow_wildcard = true # only when intentionally binding all interfaces
```

Put this in `~/.hev/config.toml`, or the file selected by `HEV_CONFIG`.
Use an explicit IP address; wildcard binds are refused unless explicitly
allowed. Seeds and advertise addresses must also be IP:port pairs. Bind port
zero is supported for isolated tests. Keep machine names stable and unique.
`name` allows up to 64 bytes of letters, digits, dots, dashes and underscores.

Open the chosen port for **both TCP and UDP**, only on the private interface
and only between your machines. This does not configure a firewall, discover
network addresses, or start a listener for `hev nodes` SSH invitations.

## Key creation and rotation

Create 32 random bytes, encode them with standard base64, and store that value
in your secret manager (for example generate with `openssl rand -base64 32`
in a trusted terminal). Configure the neutral `secret://` alias using the
[host secrets manager](secrets.md), or an explicit supported store reference.
Use the same key on all members. Do not place its value in TOML, command
arguments, logs, git or chat. Encryption and authentication are mandatory for
all gossip and TCP state exchange; a wrong or absent key cannot join.

Hevd resolves the reference through its existing in-memory secrets cache with
a 24-hour TTL and retains the decoded key for its lifetime. Startup failure
leaves the rest of hevd running; `status` shows the failure and next retry.
Retries wait at least one minute and never precede a provider cooldown.
A loaded key is never periodically reread. Update it by restarting hevd.

For rotation today: coordinate a stop of all members, replace the stored key
and update `key_ref` if needed, then restart them. This temporarily removes
membership visibility. The package exposes memberlist keyring add/use/remove
for local embedding: add the new key on every member, switch every primary,
then remove the old key everywhere. There is no remote key-management API,
key-bearing gossip or CLI rolling-rotation command in this release.

## Reading membership

```sh
hev cluster ls
hev cluster ls --json
hev cluster status
hev cluster status --json
hev cluster join 192.0.2.11:7946
hev cluster leave
```

`ls` shows every known member, its address and state, kit version/channel,
OS/architecture, dispatch host flag and slot cap, and time since the last
observed transition. `alive` means responding; `dead` means its SWIM
suspicion timer expired; `left` means a graceful departure was observed.
SWIM still suspects peers internally, but stock memberlist does not expose
suspect/refute events reliably, so this CLI does not display or log them.
Graceful departure intent is startup-scoped and sent before leave over
encrypted reliable messages, gossip and push/pull; a delayed notice can
correct a transient dead classification to left.
State changes are logged, without per-probe output. Entries are retained up
to the 64-member bound. With no config table, every subcommand prints an
enable hint and exits non-zero without contacting a daemon or network.

`status` reports key **loaded or not**, never the value or reference. It also
shows enabled/running state, bind address, member counts, seed retry state,
retry timestamps and per-node counters, provider cooldown registers and meter
records. Its local endpoint uses a private Unix socket plus peer UID checks,
or a user-protected Windows named pipe with peer token checks.
`HEV_CLUSTER_ENDPOINT` selects an isolated endpoint for tests or another daemon.

A sleeping laptop eventually appears dead, which is expected. On wake,
seed retries reconnect it automatically (backoff up to 60 seconds). Restarted
members rejoin under their stable name and recover their counter component
from peers before they may increment it. Graceful shutdown sends leave.
Explicit `leave` pauses seed retries; `join` resumes membership. WAN defaults
probe every 5 seconds with a 3-second timeout and adaptive suspicion timers;
under load there is no hard detection deadline. See [RFC 0016](rfcs/0016-cluster.md)
for timings, bounds and failure semantics.

## Usage facts and boundaries

Counters are grow-only **per node and window**, merged by maximum. The API
supports incrementing or publishing a monotonic own-component total. Minute
buckets provide rolling hour/day views, conservatively overcounting by up to
one minute; bucket timestamps use the writer's UTC clock. Keep clocks synchronized
within one minute; buckets allow that skew and expire after 24 hours plus a
minute. Calendar-day windows are also supported (today and yesterday).
Sparse buckets carry non-zero entries only. A durable ledger should publish
each bucket's total, not its current rolling sum, which can decrease.
Provider retry-after registers merge by latest time. Meter records contain only
numeric quota fields, a fixed `account`/`token` label, timestamp and writer;
newest timestamp wins, with node name breaking ties. This alone does not elect
a single meter poller or enforce a strict budget.

No secret values or references travel through this seam. No paths, repository
names, item names or arbitrary metadata belong in node facts or meter records.
Members share knowledge, not authority: membership grants no dispatch or node
admission rights, no remote commands, no secrets access, no scheduling, no
updates and no consensus. Teammates' accounts on shared hosts are outside an
operator's cluster. `hev dispatch hosts` still uses its existing probes;
cluster liveness may become a hint for it in a later change. Secrets budgets
and cooldown behavior are unchanged in this increment.

The counters are in memory and eventually consistent. Restarting one member
recovers state from surviving peers; stopping every member loses it. A
partition can temporarily undercount, and a restart must reach a seed before
incrementing. Do not treat this seam as durable accounting or a strict quota.

## Node capacity and health

`hev cluster ls` adds SESSIONS (working/total), SLOTS (free/cap with FIFO queue
in parentheses), BUILD (short revision, with a staged flag) and HEALTH.
The totals count alive members, their fresh sessions and fresh free slots.
A dash means the section is absent; a node without dispatch configuration has
no dispatch section. `stale` means the section's observation is older than three
one-minute refresh intervals. Stale counts are excluded from totals. HEALTH is
`ok` or a list of failing/stale sections; it is not a guarantee of availability.

```sh
hev cluster facts
hev cluster facts example
hev cluster facts example --json
```

Facts show every available section with independent ages in seconds. JSON `ls`
keeps its existing array and member fields and adds `facts`, `ages` and `stale`;
`status --json` adds the same facts and age/stale maps keyed by node. The
`counters` field also retains the full replicated seam. No scheduling consumer
has been switched to this view.

Credential health is observed, never probed. `unknown` means no usable local
observation exists, including installed harness logins that cannot be checked
without launching them. Provider states reflect the cache's last real request;
Layer states reflect upload authentication. A throttled provider request proves
the credential reached the provider and is not itself an authentication failure.
A credential item's timestamp is when work observed it; an old observation does
not become current just because the node refreshed its record. Capture backlog
counts source files whose signatures have not been committed by indexing, from
the existing scan enumeration. Capture error classes contain no raw diagnostics.
The updater has no local drain mode yet; no draining field is published. No gossip
record can cause updates, drain, restarts or dispatch.

Facts collectors sample at most once every 30 seconds, publish changes at that
cadence, and refresh unchanged facts every minute. Every published time is UTC.
Credential `rate-limited` means the provider was observed in cooldown; the health
column shows `credentials:rate-limited`. No health sampling consumes provider quota.
A configured Layer key reference is `unknown` until real upload work observes it.
Capture `running` means a capture cycle completed within the last three configured
scan intervals; daemon startup alone does not establish capture health.
`cluster status` reports `Facts rejected` (`facts_rejected` in JSON), a local
process-lifetime count of failed facts publications, without per-failure logging.
