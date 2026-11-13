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
entries live only in daemon memory and are discarded on restart. No value
persistence, keychain, desktop-app unlock, temp file, or stale-after-TTL behavior exists.
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
cache; launch a daemon to protect concurrent or repeated runs. The rate-limit
cooldown alone is shared across processes (see "Host cooldown" below).
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

**Endpoint, provenance and fallback.** `HEV_SECRETS_ENDPOINT` overrides the
endpoint on every OS, honoured by `Serve` and by every client: an absolute
Unix socket path (lock file beside it) or a local `\\.\pipe\NAME` pipe with the
same protected DACL and peer checks; the default is unchanged. A resolution
returns each reference's disposition, `cache` or `read`, and the path taken,
`daemon` or `direct` (`secrets.ResolveResult`; `--explain`, `env --format
json-detail`); dispositions never carry values. A direct fallback is never
silent: it prints `secrets.FallbackNotice` on stderr, and `--direct-status`
makes `read` and `env` exit 78 after delivering the value. `hev secrets ping`
and `status --no-quota` answer whether the endpoint is up with no 1Password
request; a daemon that predates `Ping` reads it as an empty resolution, which
is also free.

Staged host changes (wrappers, the shell loader, units and launchd jobs) live
in [`staged/host-secrets`](../../staged/host-secrets/) with install and undo
steps; none is applied by kit.

**Host cooldown.** A cooldown held only in one process's memory was lost
exactly when it mattered: hevd exited when its Layer-key read was rate
limited, and its supervisor restarted it into one more request per start
(win-oven logged 398 failed starts on 2026-11-11); a direct-fallback client in
a loop started a fresh cache and a fresh request each time. Now the first
rate-limit answer is persisted to `op-cooldown.json` beside the read log
(`HEV_SECRETS_COOLDOWN` moves it; `off` disables it), mode 0600 in a 0700
directory, written atomically. It holds the limit class, the reset time and
the count of consecutive unclassified limits, and nothing else: no value,
token, reference or label. Before any request (an inject batch, a quota
check, kit's direct Layer-key `op read`, the native gate's direct `op read`)
the process reads it, and inside the window answers with the same rate-limit
error and makes no request. A file that is malformed, names an unknown class
or resets more than 24 hours ahead is ignored, so it cannot block reads past
a limit's window. The first successful request after the reset removes it;
a later reset written meanwhile by another process is kept. The rule against
persistence is about values; a reset time is not a secret.

**hevd on a rate-limited Layer key.** hevd starts its secrets endpoint before
it reads the Layer key. On a rate-limit answer it no longer exits: it logs the
reset time, keeps serving the endpoint (cache hits and the cooldown answer for
every other caller) and waits until the reset, then tries once more; a stop
signal ends the wait cleanly. Exiting could not get the key sooner, would drop
the endpoint every caller relies on, and would hand the supervisor a restart
loop. With the cooldown file even that loop would cost no requests, but a unit
that stays active also stops the failed-start noise that hid the cause on
2026-11-11. Any other Layer-key error still ends hevd as before.

## Providers: Azure Key Vault beside 1Password

Added in November 2026 so a host whose secrets live in Azure Key Vault (a
Windows host among them) uses kit exactly as a 1Password host does. Every rule
above is unchanged and now holds per provider: memory only, TTL at most 24
hours, no stale-after-TTL, values never on disk, in logs, in arguments or in
errors, one attempt on a rate limit, a persisted cooldown, a mixed request
failing as a whole.

**Seam.** `internal/secrets.Provider` is the interface the cache resolves
through: `Scheme`, `Name`, `Kind` (the read-log kind), `Credential` (the
credential type, or an error naming what to set; no request), `Fingerprint`
(for the endpoint's account check) and `Read` (one attempt per unique
reference, returning values in order and the number of real requests; a
`*RateLimit` for a throttle). The cache keeps everything else: entries,
TTLs, counters, miss coalescing (one fetch lock per provider, so a slow or
throttled vault never holds a 1Password read), cooldowns and the read log. A
provider registers itself from its own file with its scheme, its reference
validator and its constructor; a third provider is one new file. 1Password
(`onepassword.go`) is the original `op inject` path moved behind the seam,
unchanged.

**References.** `azkv://VAULT/SECRET[/VERSION]` names
`https://VAULT.vault.azure.net/secrets/SECRET[/VERSION]`. A sovereign cloud's
vault is written with its full host, `azkv://VAULT.vault.usgovcloudapi.net/…`
(also `vault.azure.cn`, `vault.microsoftazure.de`); the token audience is
`https://SUFFIX` and the default sign-in host is that cloud's. Other suffixes
(Azure Stack Hub) must be listed in `HEV_AZKV_DNS_SUFFIXES` and need
`AZURE_AUTHORITY_HOST`. A private endpoint changes DNS, not the name, so it
needs nothing. Validation is Key Vault's own grammar: vault 3-24 letters,
digits and single hyphens, starting with a letter and ending alphanumeric;
secret 1-127 letters, digits and hyphens; version 32 hex digits; nothing else
(no port, userinfo, query, fragment, percent-encoding, empty or extra
segment). The `op://` checks are unchanged, and both share the generic
refusal of control characters and template braces.

**Client: REST over `net/http`, not the Azure SDK.** kit is a public single
binary. `azidentity` + `azsecrets` would add `azcore`, MSAL for Go, a JWT
library and a browser launcher (for the interactive credentials this RFC
forbids), several megabytes and a dependency surface to track, to make one
GET per secret and one token POST. Windows behaves the same either way
(`net/http`, no cgo; `az` resolves to `az.cmd` through `PATHEXT`). Each
credential type is a few lines of documented protocol, all covered against a
local fake: client secret and certificate (an RS256 client assertion with the
`x5t` thumbprint, signed with the standard library) and workload identity
(a federated token file) as OAuth client-credentials POSTs; managed identity
on a VM (instance metadata), App Service (`IDENTITY_ENDPOINT` +
`IDENTITY_HEADER`) and Azure Arc (`IDENTITY_ENDPOINT` + `IMDS_ENDPOINT`, with
the challenge key file read only from the agent's token directory); and
`az account get-access-token` for an existing login. What the SDK would add
and kit does not: PFX and password-protected certificates, the challenge
flow that discovers a vault's tenant (kit takes `AZURE_TENANT_ID`), and the
SDK's retries (which RFC 0009 forbids anyway).

**Authentication is non-interactive only.** No browser or device-code flow
exists in the code. With nothing configured the error names every variable
that would configure it. The metadata endpoint and `az` are used only when
`HEV_AZKV_CREDENTIAL` names them, so a host outside Azure never probes and
never borrows a developer's `az login` by accident; status labels `azure-cli`
as a convenience. Access tokens live in the resolving process's memory,
per audience, until two minutes before expiry; a 401 drops the cached token.
Bearer tokens are never sent across a redirect, and test or local identity
endpoints must be loopback.

**The principal's own secret.** A secret-free credential comes first: managed
identity (Azure VM or Arc-connected server) or workload identity; then a
certificate (a key file readable only by the user, the same class as the
1Password token file). A client secret is provisioned as a reference,
`AZURE_CLIENT_SECRET_REF=op://…`, which the daemon reads through its own
cache, in memory, with a one-hour TTL; never in a unit file, task or profile.
A reference back to `azkv://` is refused when configured, and a run-time
chain carried in the request context refuses any longer loop, so credential
reads always form a chain that ends.

**Endpoint accounts.** A request carries one fingerprint per configured
provider (the 1Password one also in the old `Account` field, so daemons
and clients of either age interoperate for 1Password). A resolution is
served when every provider it names matches; ping and status when any does.
An Azure fingerprint covers credential type, tenant, client, the SHA-256 of a
client secret or the secret's reference, certificate and token-file paths,
the identity endpoint and the authority. A daemon that predates providers
refuses `azkv://` references, so hevd is upgraded first.

**Throttling and cooldown.** Key Vault throttles with HTTP 429 and
`Retry-After`. kit makes that one request, stops the batch, and honours
`Retry-After` exactly (seconds or an HTTP date, 1 s to 24 h); without it, the
unclassified backoff. The cooldown goes through the same persisted host
cooldown as 1Password, keyed per provider: 1Password keeps
`op-cooldown.json` (the native gate reads and writes it), Key Vault uses
`azkv-cooldown.json` beside it, same shape. A request that mixes providers
checks every provider with a miss before asking any, so it fails atomically.
Vault errors are sanitized to the status and a fixed explanation (403 says
the principal needs get on secrets); bodies, which may echo the request, are
discarded.

**Read log and status.** Each line gains `provider=op|azkv` at the end, so
`awk '{print $2}'` still yields the caller; a line without it is 1Password.
A Key Vault batch is `kind=get`. Labels stay variable name plus a SHA-256
prefix of the reference: safe for Key Vault for the same reason as for
1Password, and a vault name is public DNS anyway. Status adds one row per
provider: configured or not (and what to set), credential type only,
reachability from the last real request, hits, misses, requests (plus token
requests) and cooldown. Key Vault has no account quota endpoint, and status
says so instead of a number.

**Call sites.** `hev secrets run|read|env` accept either scheme, mixed in one
invocation. The Layer key reads `HEVLAYER_API_KEY_REF` (either scheme) and
still `HEVLAYER_API_KEY_OP_REF`; with no daemon a Key Vault key is read
directly by kit's own client. The native gate accepts `azkv://` credential
references; with no daemon it asks `hev secrets env` to read them directly,
and never hands one to `op`.

**Validation.** Tests use a local fake vault, a fake token endpoint and fake
identity endpoints over loopback HTTP, a fake `az` on a test PATH, and the
existing fake `op`. No Azure tenant or vault is contacted. Not verified
without a real vault: the exact token and error shapes of each Azure endpoint
beyond their documentation, Arc's challenge on a real agent, sovereign-cloud
audiences, and 429 behaviour under real load.
