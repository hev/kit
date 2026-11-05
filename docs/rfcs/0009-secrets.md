# 0009: Local memory-only service-account secrets

Status: proposed, implemented for review.

`hev secrets run --ref TOKEN=op://example/service/token -- command args` uses
Capture's local secrets endpoint. Default TTL is five minutes; repeated
`--ttl TOKEN=30s` configures a reference's freshness, and `--ttl TOKEN=0s`
requires a new read. TTL is bounded at 24 hours. Shorter caller TTLs are
honored. No host rollout is included in this change.

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
the last classified rate error and retry-after time. It queries
`op service-account ratelimit --format=json` when available and outside cooldown,
exposing only numeric quota fields under known labels. Raw output is discarded.
Counters are process-local, count attempted inject batches, and reset on restart.

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
