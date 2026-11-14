# RFC 0016 — Encrypted membership and replicated usage facts in hevd

## Summary

An optional `[cluster]` table starts HashiCorp memberlist (SWIM with Lifeguard)
inside hevd. Membership is eventually consistent knowledge, never authority.
There is no leader, consensus, new daemon, remote execution or secret replication.

## Motivation

Independent daemons cannot see shared usage or a sleeping machine's liveness.
Membership and a bounded counter seam support the secrets account budget,
shared cooldowns and a single meter host.

## Decision

Use memberlist v0.5.3 (MPL-2.0) with WAN defaults: probe interval 5s, probe
timeout 3s, suspicion multiplier 6, suspicion maximum multiplier 6,
indirect checks 3, awareness maximum multiplier 8, gossip interval 500ms,
push/pull interval 60s. We shorten WAN TCPTimeout from 30s to 3s to bound
a sleeping peer's impact on seed attempts and concurrent graceful notices.
These adapt under load: there is no unconditional
wall-clock failure bound on an overloaded or partitioned network. For three
healthy observers, an idle dead peer is normally detected within 60 seconds;
the maximum suspicion timer alone is 180 seconds for three members
(6 × 6 × max(1, log10(3)) × 5s), before Lifeguard awareness scaling.
The loopback acceptance profile uses 100ms probes, 40ms timeout, suspicion
multiplier 2 / maximum 1, awareness 1, and a 5s observation bound. Tests require both surviving members to
report dead, then automatic rejoin. The stock-library acceptance measured
456ms on loopback; this is not a production WAN guarantee.

Gossip and stream push/pull require encryption verification both ways and a
32-byte AES-GCM key. `key_ref` resolves through the existing secrets cache,
TTL 24h, once on successful startup; the cluster retains it in memory for its
lifetime. Failures retry no sooner than the provider's cooldown or one minute,
whichever is later. Neither values nor references enter status or logs.
Malformed config reports only a line number. An absent table creates no
cluster listener, including no local IPC listener.

Seed join attempts back off from 2s to 60s and continue forever even after a
successful join. This repairs partitions and sleep/wake. Graceful shutdown
sends leave before shutdown. Explicit CLI leave pauses automatic seed joins
until CLI join or daemon restart. CLI join after leave recreates memberlist
with a fresh startup timestamp while preserving the counter state. State changes
are retained in an event-driven table. We never inspect upstream `Node.State`,
which is shadowed by its internal state field. `NotifyJoin` marks alive;
`NotifyLeave` marks dead unless an authenticated departure-intent notice for
that member's startup timestamp is known. Before Leave, send the notice to
known alive peers over reliable memberlist messages and enqueue it in gossip;
full push/pull also carries the bounded departure map. Late notices correct
dead to left, while notices for an earlier startup cannot affect a rejoin.

**Amended decision (Adam, 2026-11-13): stock memberlist, no fork.** Suspect
visibility and refute logging are dropped. Stock callbacks do not expose
suspicion or accepted refutation; parsing logs or using AliveDelegate is
unreliable (the latter runs before incarnation checks). Alive/dead/left,
detection bounds and automatic rejoin remain required. Internally memberlist
still uses SWIM suspicion, indirect probes and Lifeguard. We log join, dead
and observed graceful leave once per transition, never probes. A tiny optional
state callback would be upstreamable, but no patch is shipped or required.

## Goals / Non-goals

Make every member know the others and publish only fixed, versioned facts:
name, kit version/channel, OS/arch, daemon start time, dispatch enabled/cap.
No paths, repositories, secret references, credentials or arbitrary user
metadata may be added. One operator's own machines belong in a cluster;
a teammate's account on a shared host does not. Membership grants neither
`hev nodes` admission nor `dispatch.toml` scheduling rights. Dispatch may
later consult cluster liveness as a hint instead of SSH probes; no change now.

## Vocabulary

A member name is a stable per-machine identity. A window is a UTC minute bucket (`YYYY-MM-DDTHH:MMZ`) or calendar
day (`YYYY-MM-DD`). Writer clocks should agree within one minute. A register is a per-name maximum UTC timestamp.

## Proposed surface

See [cluster operations](../cluster.md) for config and CLI. Authenticated local
IPC uses private Unix sockets with peer UID checks or Windows named pipes
with protected user DACLs and peer token checks, like secrets IPC.

Counters are per-name, per-window, per-member G-counters merged by maximum.
A restarted member recovers its own component from peers before incrementing;
Add is refused until an initial successful seed exchange (unless seedless).
Losing every member loses the in-memory counters; they are not durable billing.
Retain minute buckets for 24 hours plus one minute of skew tolerance and
accept buckets up to one minute ahead. Also retain current/previous UTC day.
Rolling hour/day views sum minute buckets, conservatively overcounting by up to
one minute, rather than declining rolling totals. Sparse `SetOwn` publication ignores
zero and cannot lower a component within a bucket. Refuse older/future windows. Clock
skew can split daily counts, so callers should consult both days near midnight;
this seam cannot enforce a strict account quota or correct clocks. Registers
keep the latest timestamp, independent of writer clocks; operators need clock
synchronization. Maximums: 32 counter names, 1445 windows/name (minute buckets plus day keys),
64 nodes/window, 32 registers; identifiers at most 64 bytes. Full state is at
most 1MiB, at most 256KiB per name, and 96KiB estimated entry size per writer.
Broadcasts are individual component/register/meter deltas, not full snapshots.
Provider cooldown registers use `1password` and `azure-key-vault` names.
A last-writer-wins meter has only uint64 quota fields, `account`/`token` label,
UTC timestamp and writer node. Only the two provider names above are accepted;
newer timestamp wins, with lexical node tie-break. Meter readings do not
elect a poller. All these facts appear in status.
Invalid or oversized merges are rejected atomically. Expired windows are
pruned before operations/merges, so delayed gossip cannot resurrect them.

Key rotation uses memberlist's keyring add/use/remove in the package API.
Distribute the new key securely to each member's local keyring, switch every
member's primary, then remove the old key everywhere. There is deliberately no
key-bearing gossip or CLI argument. Until a protected multi-key configuration
surface is added, operators use a coordinated stop/change key_ref/restart;
rolling rotation through the internal API is for embedding, not remote RPC.

## Open questions

None for this increment. Persistence, strict quotas, and operator-facing rolling
key rotation remain follow-ups, outside this order's CLI surface.

## Secrets consumer

The daemon attaches an in-process adapter after configuring its cluster.
Reservations publish absolute per-provider minute buckets from the durable
ledger; a one-second worker includes direct local writers. The consumer uses
rolling sums while another ledger heartbeat is at most five seconds old,
retains stale peer counts and reports every peer's age, and otherwise explicitly
falls back to mandatory safe host limits. No-cluster hosts retain local behavior.

The minimal seam addition is a bounded per-node last-writer-wins usage heartbeat:
timestamp plus two maps of positive daily/hourly budgets keyed only by `op` or
`azkv`, at most 64 nodes. Heartbeats expire after 24 hours plus one minute
of skew tolerance; timestamps more than one minute ahead are rejected atomically.
Absent account budgets mean no account policy; published values must be positive,
and zero is not a way to block a provider. Equal timestamps keep the existing
record. It carries
no commands, paths, references or values. The broadcast splitter forwards this
new fixed metadata delta; membership and transport are unchanged. Identical
local account policies are compared and mismatches displayed. Cooldown registers
are keyed by provider and a bounded hash of the source node; the winning peer
is identified in status and read logs. The fixed account meter includes its
source and age. Only the designated meter host evaluates account warnings,
with persisted crossings, while all hosts evaluate host warnings.

See [the account budget example](../secrets.md#shared-account-budget-across-a-cluster)
for four hosts on 1,000/day, safe partition budgets, the conditional overshoot
bound `sum(B_i + r_i * D)`, and the unexplained-spending figure. Gossip has no
unconditional delivery bound: during isolation only host caps apply. The
rolling consumer now retains the boundary minute, overcounting by up to
one minute to avoid prematurely freeing budget.

## Node facts (schema 1)

The counter seam also carries at most 64 node records, each at most 4096 bytes
of JSON. A node writes only its own record; its strictly newer timestamp wins.
Each optional section has an independent observation time: dispatch capacity
(cap/running/free/FIFO queue), sessions (working/idle by the five fixed harness
labels and foh/boh), build (short hexadecimal revision, staged and last
check outcome), credentials (op/azkv/Layer and installed harnesses, fixed health
labels), and capture (running, last upload, pending file count and error class).
No version/channel duplication is added to memberlist metadata.

Unknown record fields or enum labels, oversized records, and timestamps more
than one minute ahead are rejected atomically with the containing merge.
Records expire after 24 hours plus one minute; delayed snapshots cannot revive
expired records. The existing full state limit remains 1MiB. Deltas carry a
single record, compressed with zlib behind a fixed byte tag to fit the UDP
budget; decompression is bounded to the record envelope size. Full push/pull
keeps JSON. This is a bounded encoding addition to the seam, with no membership,
transport scheduling, encryption or timer change. Old binaries ignore the new
facts in full state and cannot interpret their delta tag.

Publication samples local observations every thirty seconds, sends changes and
refreshes at least once per minute. A section older than three minutes is stale.
Credential item times retain the time of real work, rather than being renewed
by a publication. Provider observations come from the existing cache status;
Layer authentication comes from capture upload outcomes. Installed harnesses
remain unknown: finding an executable is not evidence of login validity.
No provider request or harness launch is made to collect health. The updater
currently has no drain lifecycle. A future draining field is reserved here but
is absent from schema 1 and the CLI until kit has a real local observation.

The gossip rule applies equally to all five cases: only bounded numbers,
booleans, times and whitelisted labels (plus the bounded hexadecimal build
identifier) may cross the seam. Session names, paths, repositories, account or
key names, secret references and upstream errors are never facts. Capacity is
knowledge, not permission to schedule; the staged flag authorizes no action.

Node facts sampling is bounded to once per 30 seconds, with publication on sampled
change and a one-minute refresh. All published times are UTC, and the record time
includes concurrent last-check and last-upload observations. Failed local facts
publications increment the local status `facts_rejected` counter without logging.
The credential state whitelist is `ok`, `failing`, `unknown`, `rate-limited`;
the last indicates an observed provider cooldown. Configured Layer references
start as `unknown`. Capture `running` requires a completed capture cycle in the
last three scan intervals, rather than merely a live daemon process.
