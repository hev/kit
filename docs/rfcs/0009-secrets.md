# 0009: Local memory-only service-account secrets

Status: proposed, implemented for review.

`hev secrets run --ref TOKEN=op://example/service/token -- command args` uses
Capture's local secrets endpoint. Default TTL is five minutes; repeated
`--ttl TOKEN=30s` configures a reference's freshness, and `--ttl TOKEN=0s`
requires a new read. TTL is bounded at 24 hours. Shorter caller TTLs are
honored. The first change included no host rollout; [Host rollout](#host-rollout)
below adds what every host needs to read only through the cache, and
[How a host reads secrets](../secrets.md) is the procedure.

## Trust and storage

Values must never be written to disk, logs, command arguments or trace
archives. Templates go to `op inject` on stdin; results return through an
anonymous pipe, authenticated local IPC, and the child's environment. Cache
entries live only in daemon memory and are discarded on restart. No persistence,
keychain, desktop-app unlock, temp file, or stale-after-TTL behavior exists.
A service-account token in the resolver's environment is required, and biometric
unlock is disabled for all op subprocesses. Provision that environment through
the existing secret-aware launcher; never put a token in a unit file.

Unix sockets live under a private 0700 directory, have mode 0600, and both ends
check kernel peer credentials. Windows uses a per-SID named pipe, a protected
DACL granting only that SID, remote-client rejection, and peer process token
checks in both directions. The endpoint is a same-user trust boundary: another
process of the same user can request secrets. Administrators can inspect process
memory regardless. Environment inspection by the same user remains possible.
Go strings cannot guarantee erasure of freed memory; swap and crash dumps are
outside the application's control. Operators requiring RAM-only physical
storage must disable dumps and use encrypted swap or disable swap.

Kit does not log templates, values, reference URIs, or raw resolver stderr.
Existing archive redaction continues to apply. Children must consume values
from the environment and must not echo, persist, or pass them as arguments.
The runner cannot prevent arbitrary child code from doing so; applications that
log their environment must not be used. Redaction is defense in depth, not a
substitute for this requirement.

## Resolution and failure policy

A resolver-wide mutex coalesces concurrent misses and serializes batches.
Each batch deduplicates references and uses one `op inject` call, framed with
a random boundary that supports multiline values. A fresh request bypasses
cache reuse. Mixed-TTL aliases retain the shortest storage expiry. The mutex
also prevents concurrent requests from generating a retry storm.

Rate-limit errors are classified as daily, hourly or unspecified. No immediate
retry occurs: a shared cooldown adds jitter and uses the reset timestamp when
op supplies an RFC3339 UTC reset. Otherwise the conservative retry-after estimate
is 24 hours, one hour or one minute respectively. Errors name the class and
retry-after time without including raw op diagnostics. The estimate is not a
claim about the upstream account's exact reset. Requests wholly covered by
unexpired cache continue during cooldown; requests requiring missing values
fail atomically. Expired values are never served. Authentication and other
failures produce one sanitized error. Desktop fallback is prohibited.

When the endpoint cannot be connected, the CLI resolves directly with the same
batching and error policy for that invocation. Direct mode has no cross-process
cache or cooldown; launch a daemon to protect concurrent or repeated runs.
A connected endpoint's protocol, authorization or resolver error never triggers
a second direct request. A daemon can be bootstrapped through direct mode.

`hev secrets status` reports hit/miss counts, batch reads in the last hour/day,
real requests since start, the last classified rate error and retry-after time,
and the account's quota rows. It queries `op service-account ratelimit
--format=json` when available, outside cooldown and at most every five
minutes, exposing only numeric quota fields, whitelisted row labels and a reset
time. Raw output is discarded. Counters are process-local, count attempted
requests, and reset on restart; the host read log (below) covers every process.

## Integration examples

These are examples only; deployment follows review. Use example references and
adapt existing launchers without saving their resolved environment.

Dispatch `harness_wrapper` (an executable script):

```sh
#!/bin/sh
exec hev secrets run --ref API_TOKEN=op://example/service/token -- "$@"
```

Daemon launcher:

```sh
exec hev secrets run --ref LAYER_API_KEY=op://example/archive/token -- hev daemon
```

For systemd use an `ExecStart` pointing at the command below. For launchd put
the same separate arguments in `ProgramArguments`. For a Windows scheduled task
set Program to `hev.exe` and put the remaining arguments in its argument field.
Do not use shell expansion of the resolved value in any scheduler definition.

```text
hev secrets run --ref JOB_TOKEN=op://example/job/token --ttl JOB_TOKEN=30s -- job-worker
```

## Validation

Tests use only a fake op implemented by the test executable. Coverage includes
concurrent reuse, batching, expiry, fresh reads, rate cooldown and cached service,
sanitized errors/status, local IPC, direct fallback, UID comparison and Windows
pipe ACL construction. Native Windows CI runs the same IPC and fake-op tests.
No real account is read by validation. A separate secrets-only endpoint mode
allows isolated acceptance without starting archive ingestion.

### Review hardening

The endpoint captures a SHA-256 account fingerprint at startup; requests carry
only the caller's fingerprint. A missing or differing resolver account yields a
distinct unavailable response and the caller resolves directly with its own
environment. Values are never reused across accounts. Endpoint startup failure
is nonfatal to Capture, including an already-running secrets-only endpoint.

Rate-limit classification strips reference tokens and requires explicit limit
wording. Reset timestamps must follow reset/retry wording and fit within the
class's conservative delay. Status snapshots counters before invoking `op`, so
slow quota inspection cannot block hits. Array quota rows use stable numeric row
prefixes to preserve distinct rows without publishing arbitrary string labels.
The exact upstream JSON shape remains unverified: validation uses fake `op` only.

On Unix, `secrets run` replaces itself with the launched program, preserving PID,
signals and exit semantics. Windows retains native child execution. Linux daemon
stop only signals matching processes owned by the current user whose HOME and
HEV_CONFIG match the caller. Uninspectable environments and relative explicit
config paths fail closed. Other Unix platforms rely on the scoped service
manager rather than signaling processes from a host-wide name scan.

## Host rollout

Added in November 2026, after the shared account ran out on 2026-11-10 and every
host lost its secrets for a day. The rules above are unchanged: memory only, no
persistence, TTL at most 24 hours, no value served after its TTL.

**Token file.** The resolver and the account fingerprint use
`OP_SERVICE_ACCOUNT_TOKEN`, or else the token file `HEV_OP_TOKEN_FILE`
(default `~/.config/op/token`), read at use time and only when its mode
grants nothing to group or others. Before this, hevd units that relied on
the file (as kit's Layer-key reader did) ran an endpoint that answered every
request "unavailable". The token still reaches `op` only in the child's
environment.

**Shell call sites.** `hev secrets read REF` prints one value to stdout for
`x=$(hev secrets read REF)`; `hev secrets env --ref NAME=REF ...` prints
`export NAME='value'` lines for `eval` in the calling shell (or JSON with
`--format json`), resolving every reference in one batch. Values travel on the
pipe a command substitution already uses; nothing is written to disk and no
value is a process argument. `--cache-only` exits 69 when no endpoint serves
the account and 75 for any endpoint answer; without it a rate limit exits 75.

**kit's own reads.** The Layer key (`HEVLAYER_API_KEY_OP_REF`) and the native
gate's database references go through the endpoint with a six-hour and
one-hour TTL respectively, in one batch for the gate. With no endpoint for the
account they read directly exactly as before. An endpoint's answer, a rate
limit included, is final and never retried directly. `HEV_SECRETS_CACHE=off`
skips the endpoint for a process that must use a fixture `op` on its PATH.

**Read log.** Each real request appends one line to
`~/.hev/secrets/op-reads.log` (0600 in a 0700 directory; `HEV_SECRETS_READ_LOG`
moves or disables it): UTC time, caller, kind, reference count, labels and
outcome. A label is the caller's variable name and the first eight hex digits
of the reference's SHA-256, so the log carries no value, token, template or
reference URI. The caller is `--caller`, `HEV_SECRETS_CALLER`, or the parent
command name. Direct reads by kit and by the native gate log the same way.

**Rate limits.** Reset timestamps are now matched case-insensitively; the
earlier pattern ran against lower-cased text and never matched, so op's own
reset time was ignored. An unclassified limit ("Too many requests", as on
2026-11-10) may be the daily quota, so consecutive unclassified answers double
the cooldown from one minute up to one hour, and an explicit reset time or
"try again in N" up to 24 hours ahead is honoured for it. Every rate-limit
answer is one attempt, logged with outcome `rate_limited`.

Staged host changes (wrappers, the shell loader, units and launchd jobs) live
in [`staged/host-secrets`](../../staged/host-secrets/) with install and undo
steps; none is applied by kit.
