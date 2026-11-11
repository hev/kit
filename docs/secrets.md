# How a host reads secrets

Every 1Password read on a host goes through kit's local secrets cache. A host
shares one service-account quota with every other host (1,000 requests a day
for the account), so a read that skips the cache spends quota that every
other host also needs. This page is the procedure; [RFC 0009](rfcs/0009-secrets.md)
is the design and security boundary.

## The one rule

Read through `hev secrets`. Do not run `op read`, `op inject` or `op run`
yourself, in a script, a wrapper, a shell profile or an agent session.

| You need | Use |
| --- | --- |
| One value in a shell variable | `x=$(hev secrets read --label NAME --ttl 1h op://vault/item/field)` |
| Several variables in the current shell | `eval "$(hev secrets env --ttl A=1h --ref A=op://… --ref B=op://…)"` |
| Values in a program's environment | `hev secrets run --ref A=op://… --ttl A=1h -- program args` |
| Values in a Go or other program, as JSON on a pipe | `hev secrets env --format json --ref A=op://…` |
| To know what the host is spending | `hev secrets status` |

All of them return the value through a pipe or the child's environment. None
writes a value to disk, a log or a process argument; the reference is the only
argument. `--ttl` is how long the daemon may reuse the value (default five
minutes, maximum 24 hours, `0s` forces a fresh read). Pick the longest TTL a
rotation can tolerate: an hour is a good default for API keys.

`read` and `env` exit 75 on a rate limit and print the retry time on stderr.
**Make one attempt.** Do not loop, sleep-and-retry, or fall back to `op`: the
cache already refuses to send another request before the retry time, and a
second path around it is how one host empties the account for everyone.

`--cache-only` (for programs with their own fallback) never reads directly:
it exits 69 when no daemon serves this account and 75 for any answer from the
daemon.

Never put a bulk loader (a file that resolves many secrets) in a shell
profile. Source it in the job that needs the values.

## Switching the cache on

The cache is the memory-only endpoint inside `hev daemon` (hevd). It serves a
caller when both hold the same service-account token: from
`OP_SERVICE_ACCOUNT_TOKEN`, or else the token file `HEV_OP_TOKEN_FILE`
(default `~/.config/op/token`, which must be mode 0600). So a host whose
token lives in `~/.config/op/token` needs no configuration: start or restart
hevd with this version of kit. To name another file, add one line to the
service:

```ini
# ~/.config/systemd/user/hevd.service.d/secrets.conf
[Service]
Environment=HEV_OP_TOKEN_FILE=%h/.config/op/token
```

A host without hevd runs the endpoint alone with `hev daemon --secrets-only`
(staged unit and launchd job in [`staged/host-secrets`](../staged/host-secrets/)).
`HEV_SECRETS_CACHE=off` makes a client skip the endpoint, for a process that
must use a fixture `op` on its own PATH.

With no endpoint, `hev secrets` still batches and logs, but every process pays
its own request. `hev secrets status` says `daemon: unavailable` when this
account has no endpoint.

## The read log

Every real request (an `op inject` batch, an `op read` by kit's direct
fallback, a quota check, and with the staged `with-op` every `op` command it
passes through) appends one line to `~/.hev/secrets/op-reads.log`:

```text
2026-11-11T21:31:02Z caller=bash[4121]>hev_secrets_read kind=inject refs=1 labels=DB_PASSWORD#9f2c11ab outcome=ok
```

Time (UTC), caller (`--caller`, `HEV_SECRETS_CALLER`, or the parent command),
kind, how many references, labels, and outcome (`ok`, `error`,
`rate_limited`, `passthrough`). A label is the variable name plus the first
eight hex digits of the reference's SHA-256; find it with
`printf %s op://vault/item/field | sha256sum | cut -c1-8`. The log never holds
a value, a token or a reference. It rotates to `op-reads.log.1` at 4 MiB.
`HEV_SECRETS_READ_LOG` moves it; `off` disables it.

When the account runs out:

```sh
hev secrets status                                   # quota rows, reset time, cooldown
awk '{print $2}' ~/.hev/secrets/op-reads.log | sort | uniq -c | sort -rn | head
```

## Status

`hev secrets status` (or `--json`) shows whether the daemon is up and since
when; cache hits and misses; real requests since start, batches in the last
hour and day, and how many were rate-limited; the active cooldown and its
retry time; the host read log's 24-hour count; and the account's quota rows
(type, action, limit, used, remaining, reset) from `op service-account
ratelimit`, asked at most every five minutes and never during a cooldown.
