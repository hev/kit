# How a host reads secrets

Every secret read on a host goes through kit's local secrets cache, whether
the secret lives in 1Password or Azure Key Vault. Write a reference as
`secret://NAME` and the host's secret manager decides which store it is read
from (see [The host's secret manager](#the-hosts-secret-manager)); a secret
that must name its store is written `op://vault/item/field` or
`azkv://vault/secret`. A host shares one 1Password service-account quota with
every other host (1,000 requests a day for the account), so a read that skips
the cache spends quota that every other host also needs; a vault throttles
per request. This page is the procedure; [RFC 0009](rfcs/0009-secrets.md) is
the design and security boundary.

Everything below applies to both providers: anywhere this page shows an
`op://` reference a `secret://` or `azkv://` one works the same way, and one
command may mix them. See [Choosing a provider](#choosing-a-provider) for Key Vault.

## The one rule

Read through `hev secrets`. Do not run `op read`, `op inject` or `op run`
yourself, in a script, a wrapper, a shell profile or an agent session.

| You need | Use |
| --- | --- |
| One value in a shell variable | `x=$(hev secrets read --label NAME --ttl 1h op://vault/item/field)` |
| Several variables in the current shell | `eval "$(hev secrets env --ttl A=1h --ref A=op://… --ref B=op://…)"` |
| Values in a program's environment | `hev secrets run --ref A=op://… --ttl A=1h -- program args` |
| A `.env` file of references, in a program's environment | `hev secrets run --env-file .env -- program args` |
| Values in a Go or other program, as JSON on a pipe | `hev secrets env --format json --ref A=op://…` |
| To know what the host is spending | `hev secrets status` (`--no-quota` to ask 1Password nothing) |
| To know whether the cache is up, for free | `hev secrets ping` (exit 0 up, 69 not) |
| Each value's provenance, for a wrapper's log | `--explain` on stderr, or `env --format json-detail` |

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

## The host's secret manager

`secret://NAME` names a secret without naming a store. NAME is 1 to 127
lowercase letters, digits and hyphens (`secret://stripe-api-key`); anything
else is refused. Each host sets which store such a name is read from, once, in
kit's config file `~/.hev/config.toml` (or the file `HEV_CONFIG` names), the
same file hevd reads:

```toml
[secrets]
manager = "1password"
vault = "Production"
# field = "password"   # the item field read for every NAME (default password)
```

```toml
[secrets]
manager = "azure-key-vault"
vault = "example-vault"   # or example-vault.vault.usgovcloudapi.net
```

| Manager | `secret://stripe-api-key` reads |
| --- | --- |
| `1password` | `op://Production/stripe-api-key/password`: the item titled `stripe-api-key` in the vault, field `password` (or `field`) |
| `azure-key-vault` | `azkv://example-vault/stripe-api-key`: the secret `stripe-api-key`, latest version |

`HEV_SECRETS_MANAGER`, `HEV_SECRETS_VAULT` and `HEV_SECRETS_FIELD` set the same
for one process; when `HEV_SECRETS_MANAGER` is set the file's table is not
read. With no manager set, a `secret://` read fails and says exactly what to
set; kit never guesses a manager from the credentials it finds.
`hev secrets status` shows the host's manager and where it is set.

The reference expands to its explicit form in the reading process before
anything else, so `secret://stripe-api-key` and
`op://Production/stripe-api-key/password` on that host are one cache entry,
one request, one cooldown and one read-log label. The daemon is unchanged and
only ever sees the explicit form.

**Moving a host to another manager** is a host change only: put each secret
in the new store under the same NAME, change `[secrets]`, and every `.env`,
script and `HEVLAYER_API_KEY_REF` that says `secret://` reads from the new
store on its next read. Restart hevd if its own `HEVLAYER_API_KEY_REF` is
`secret://`, since it reads the Layer key at start.

## Env files

`hev secrets run --env-file FILE -- program` and `hev secrets env --env-file
FILE` read `NAME=VALUE` lines. A value that is a reference (`secret://`,
`op://` or `azkv://`) is resolved; any other value passes through unchanged.
So a project's `.env` keeps its ordinary settings and holds references in
place of secret values:

```sh
# STRIPE_API_KEY=sk_live_...
STRIPE_API_KEY=secret://stripe-api-key
PORT=8080
```

```sh
hev secrets run --env-file .env -- ./server
eval "$(hev secrets env --env-file .env)"
```

Blank lines and `#` comments are ignored, `export ` is optional, and a value
may be unquoted (a `#` after whitespace starts a comment), `'single quoted'`
(taken literally) or `"double quoted"` (with `\n`, `\t`, `\"`, `\\` and `\$`).
A quoted value cannot span lines. If a NAME appears twice in the file the
later line wins; `--ref NAME=…` wins over the file. `--ttl NAME=1h` works for a
reference from either. Every reference in the file and on the command line is
one batch. The file is only read: values still reach the program only through
its environment or a pipe.

## Choosing a provider

The reference's scheme picks the store. Both get the same cache, TTL limit,
batching, one-attempt rate-limit rule, cooldown and read log.

| Reference | Store |
| --- | --- |
| `op://vault/item/field` | 1Password, through a service account (`op` on PATH) |
| `azkv://VAULT/SECRET` | Azure Key Vault `https://VAULT.vault.azure.net`, latest version |
| `azkv://VAULT/SECRET/VERSION` | a pinned version (32 hex digits) |
| `azkv://VAULT.vault.usgovcloudapi.net/SECRET` | a sovereign cloud's vault (also `vault.azure.cn`); the token audience and sign-in host follow the suffix |

A vault behind a private endpoint keeps its ordinary name; write it like any
other vault. An Azure Stack Hub suffix must be listed in
`HEV_AZKV_DNS_SUFFIXES` (comma separated) and needs `AZURE_AUTHORITY_HOST`.
Vault and secret names follow Key Vault's own rules; anything else (a port, a
query, an extra path segment, a trailing slash) is refused.

Use whichever store already holds the secret. A host with only one store
configured needs nothing for the other: a Key Vault-only host needs no `op`
and no 1Password token, and a 1Password-only host needs no Azure settings.

**The Azure principal needs get on secrets, nothing more:** the built-in role
**Key Vault Secrets User** on the vault (Azure RBAC), or an access policy with
only the *Get* secret permission. kit never lists, sets or deletes secrets.

Key Vault credentials, never interactive (no browser or device-code prompt
exists in kit; with none configured, the error names what to set):

| Credential | Set in the daemon's environment |
| --- | --- |
| Managed identity, Azure VM | `HEV_AZKV_CREDENTIAL=managed-identity` (`AZURE_CLIENT_ID` for a user-assigned identity) |
| Managed identity, Azure Arc server | nothing: the Arc agent sets `IDENTITY_ENDPOINT` and `IMDS_ENDPOINT`; the user must be in the agent's token group |
| Managed identity, App Service | nothing: the platform sets `IDENTITY_ENDPOINT` and `IDENTITY_HEADER` |
| Workload identity (Kubernetes) | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_FEDERATED_TOKEN_FILE` |
| Service principal, certificate | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_CERTIFICATE_PATH` (PEM: certificate and unencrypted RSA key, readable only by the user) |
| Service principal, client secret | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `AZURE_CLIENT_SECRET_REF=op://…` (read through the cache) or `AZURE_CLIENT_SECRET` |
| An existing `az login` (convenience) | `HEV_AZKV_CREDENTIAL=azure-cli`; kit runs `az account get-access-token`, never `az login` |

Selection is automatic in that order of preference for a service principal
(secret, then certificate, then workload identity, then a platform identity
endpoint); `HEV_AZKV_CREDENTIAL` names one explicitly, as one of
`client-secret`, `client-certificate`, `workload-identity`,
`managed-identity` or `azure-cli` (`auto`, or unset, selects). The instance metadata
endpoint and `az` are used only when named, so a host that is not in Azure
never waits on a metadata probe and never borrows a developer's login by
accident.

**Where the principal's secret comes from.** Prefer a credential with no
secret: a managed identity (an Azure VM or an Arc-connected server, Windows
included) or workload identity. Otherwise a certificate. A client secret must
never sit in a unit file, a scheduled task or a profile: put a reference in
`AZURE_CLIENT_SECRET_REF` (for example to 1Password) and the daemon reads it
through its own cache, in memory, for an hour at a time. A reference back to
`azkv://` is a cycle and is refused. Access tokens live only in the
resolving process's memory until two minutes before they expire.

**A Windows host that uses only Key Vault** runs hevd (or
`hev daemon --secrets-only`) with, on an Azure VM,
`HEV_AZKV_CREDENTIAL=managed-identity`; on an Arc-connected server, nothing
beyond Arc's own variables; elsewhere `AZURE_TENANT_ID`, `AZURE_CLIENT_ID` and
`AZURE_CLIENT_CERTIFICATE_PATH`. No `op`, no `OP_SERVICE_ACCOUNT_TOKEN`, no
token file. Callers then use the same commands with `azkv://` references:

```powershell
$env:DB_PASSWORD = hev secrets read --label DB_PASSWORD --ttl 1h azkv://example-vault/db-password
hev secrets run --ref DB_PASSWORD=azkv://example-vault/db-password --ttl DB_PASSWORD=1h -- program.exe
```

The Layer key takes any scheme in `HEVLAYER_API_KEY_REF`, `secret://`
included (`HEVLAYER_API_KEY_OP_REF` still works), and the native gate's
credential references may be `secret://` or `azkv://` too (read through `hev`,
which must be on PATH). `AZURE_CLIENT_SECRET_REF` may be `secret://` on a
host whose manager is 1Password; on a Key Vault host that is a cycle and is
refused.

## Knowing what happened

The fallback to a direct read is never silent. When no daemon serves the
account, `read`, `env` and `run` still deliver the value but print one line on
stderr starting `hev secrets: notice: fallback to direct`, then the stores it
reads (`1Password read`, `Azure Key Vault read`, or
`1Password and Azure Key Vault read`), with the reason and endpoint. Add `--direct-status` to `read` or `env` and they
then exit 78 instead of 0, after printing the value, so a wrapper can tell the
two paths apart by status alone. kit's Layer key and the native gate print the
same notice when they read directly.

Each reference has a disposition: `cache` (served from memory, no request)
or `read` (part of the one real request this call made). `--explain` prints
one line per reference on stderr, `hev secrets: NAME source=cache via=daemon`;
`env --format json-detail` returns `{"values":{…},"sources":{…},"via":"daemon"|"direct","endpoint":"…"}`.
Neither ever holds a value except `values`. The read log records real requests
only; a wrapper that also wants hits in its own log takes them from here.

`hev secrets ping` asks only whether a daemon serves this account: it makes no
1Password request on either side (`status` may ask for the quota; `status
--no-quota` does not).

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

The daemon serves each provider separately: a request is answered when, for
every provider its references name, the caller and the daemon hold the same
credential (the same 1Password token; the same Azure credential type, tenant,
client and secret or certificate). Only SHA-256 fingerprints cross the
endpoint. A caller whose Azure settings differ from the daemon's still gets
its 1Password reads served, and reads Key Vault directly. Upgrade hevd before
callers start using `azkv://`: a daemon that predates providers refuses those
references.

A host without hevd runs the endpoint alone with `hev daemon --secrets-only`
(staged unit and launchd job in [`staged/host-secrets`](../staged/host-secrets/)).
`hev update` restarts that unit (`hev-secrets.service`) or job
(`com.hev.secrets`) onto the new build with hevd when it is running, and
leaves a stopped one stopped. A restart sends no request, and the cooldown
file still holds.
`HEV_SECRETS_CACHE=off` makes a client skip the endpoint, for a process that
must use a fixture `op` on its own PATH.

`HEV_SECRETS_ENDPOINT` moves the endpoint for server and client alike: an
absolute socket path on Linux and macOS (its directory must be 0700; the lock
sits beside it), `\\.\pipe\NAME` on Windows (same one-user DACL and
peer-token checks). Unset keeps the default, `~/.hev/secrets/agent.sock` or
`\\.\pipe\hev-secrets-<SID>`. A test or a second daemon runs on its own
endpoint this way; set it in both the daemon's and the callers' environment.

With no endpoint, `hev secrets` still batches and logs, but every process pays
its own request. `hev secrets status` says `daemon: unavailable` when this
account has no endpoint.

## The read log

Every real request (an `op inject` batch, a Key Vault batch of secret reads,
an `op read` by kit's direct fallback, a quota check, and with the staged
`with-op` every `op` command it passes through) appends one line to
`~/.hev/secrets/op-reads.log` (the name predates Key Vault):

```text
2026-11-11T21:31:02Z caller=bash[4121]>hev_secrets_read kind=inject refs=1 labels=DB_PASSWORD#9f2c11ab outcome=ok provider=op
2026-11-11T21:31:05Z caller=bash[4121]>hev_secrets_run kind=get refs=2 labels=DB_USER#04be71d2,DB_PASSWORD#5a1c9e30 outcome=ok provider=azkv
```

Time (UTC), caller (`--caller`, `HEV_SECRETS_CALLER`, or the parent command),
kind (`inject`, `read`, `ratelimit` for 1Password; `get` for Key Vault, one
line per batch, whose sign-in and secret reads stop at the first failure), how
many references, labels, outcome (`ok`, `error`, `rate_limited`,
`passthrough`) and provider (`op`, `azkv`; a line without it was written by an
older writer and is 1Password). A label is the variable name plus the first
eight hex digits of the reference's SHA-256; find it with
`printf %s op://vault/item/field | sha256sum | cut -c1-8` (or the `azkv://`
reference). The log never holds a value, a token or a reference. A Key Vault
label is as safe as a 1Password one: it names neither vault nor secret, and a
vault name is a public DNS name anyway. It rotates to `op-reads.log.1` at 4 MiB.
`HEV_SECRETS_READ_LOG` moves it; `off` disables it.

When the account runs out:

```sh
hev secrets status                                   # quota rows, reset time, cooldown
awk '{print $2}' ~/.hev/secrets/op-reads.log | sort | uniq -c | sort -rn | head
grep 'provider=azkv' ~/.hev/secrets/op-reads.log | grep -c rate_limited   # a throttled vault
```

## After a rate limit

The first rate-limit answer starts a host-wide cooldown. Its limit class and
reset time (nothing else: no value, token or reference) go to
`~/.hev/secrets/op-cooldown.json`, beside the read log. Until that time every
kit process on the host answers with the same error and sends nothing: the
daemon, a restarted daemon, `hev secrets read`/`env` with no daemon, kit's
Layer-key read and the native gate. hevd stays up and keeps serving the
endpoint while its own Layer key waits for the reset, instead of exiting into
a restart loop. The first successful request after the reset deletes the file.

```sh
hev secrets status            # "cooldown: daily rate limit; no request from this host before ..."
cat ~/.hev/secrets/op-cooldown.json
```

The cooldown is per provider. Key Vault answers a throttled request with HTTP
429 and `Retry-After`; kit makes that one request, honours `Retry-After`
exactly (without it, the same doubling backoff as an unclassified 1Password
limit) and records it in `azkv-cooldown.json` in the same directory, in the
same shape. A throttled vault never holds back 1Password reads, and a spent
1Password account never holds back Key Vault. A request that mixes both fails
as a whole, before asking either store, when either is in cooldown.

Do not delete the file to "unstick" a host: the account is still out, and the
next request only spends more of it. A file that is malformed or claims more
than 24 hours is ignored. `HEV_SECRETS_COOLDOWN` moves it; `off` disables it
(tests only).

## Status

`hev secrets status` (or `--json`) shows whether the daemon is up and since
when; the host's secret manager for `secret://` and where it is set (in JSON,
`manager`); cache hits and misses; real requests since start, batches in the last
hour and day, and how many were rate-limited; the active cooldown and its
retry time; the host read log's 24-hour count; one row per provider; and the
1Password account's quota rows (type, action, limit, used, remaining, reset)
from `op service-account ratelimit`, asked at most every five minutes and
never during a cooldown.

```text
secret manager:  Azure Key Vault (vault example-vault; /home/me/.hev/config.toml [secrets])
providers:
  PROVIDER         SCHEME   CREDENTIAL                           REACHABLE        HITS  MISSES  REQUESTS      COOLDOWN
  1Password        op://    service-account token (environment)  yes              1     1       2             -
  Azure Key Vault  azkv://  client-secret                        yes (throttled)  1     2       2 (+1 token)  rate limit until 2026-11-11T17:02:21Z
  Azure Key Vault quota: none: Key Vault has no quota endpoint; throttling arrives per request as HTTP 429
```

A provider row gives the credential type only (never a value or identifier),
reachability from its last real request (`unknown` before the first; status
itself never asks a vault), hits, misses, real requests (Key Vault adds its
token requests) and any cooldown. An unconfigured provider says what to set.
Key Vault has no account quota to report, and status says so rather than
showing a number.

## Windows: replace a per-launch credential fetch

Keep the wrapper's engagement boundary: validate the intended account and
store ACL, reject ambient credentials, allowlist nonsecret child settings, and
select only that engagement's references. The cache replaces fetching, not
that boundary. A LocalMachine DPAPI blob can be decrypted by other accounts
on the machine if they obtain it; its protected ACL is essential.

### Configure a protected token

Both hevd and each caller use these settings, with no token in their task
arguments or environment:

```powershell
$env:HEV_OP_DPAPI_FILE = 'C:\Example\CredentialStore\token.dpapi'
$env:HEV_OP_DPAPI_SCOPE = 'LocalMachine'
$env:HEV_OP_DPAPI_ENTROPY = 'example-fixed-entropy'
Remove-Item Env:OP_SERVICE_ACCOUNT_TOKEN -ErrorAction SilentlyContinue
hev secrets token-store inspect
```

`HEV_OP_DPAPI_FILE` selects a raw binary blob written by
`.NET ProtectedData.Protect`; kit does not wrap or rewrite it. Use
`LocalMachine` for an existing machine-scoped store, `CurrentUser` for a
user-scoped one (the default). `HEV_OP_DPAPI_ENTROPY` supplies the exact
UTF-8 bytes of the original fixed entropy string; leave it unset when the
original used no entropy. The decrypted token must be UTF-8 text. A store
using different byte encoding or a base64/CLIXML envelope needs conversion
by its operator; do not point kit at an envelope or guess the entropy.
With those matching settings an existing raw store is used unchanged.

Every read checks the file and its immediate directory: both must have a
protected DACL with no inherited entries, allow access only to the current
user, SYSTEM and built-in Administrators, and have an owner from that same
allowlist. Unknown ACE types and links are refused. An unexpected principal
is named by SID. A configured store that is absent, unsafe or cannot decrypt
fails closed; kit does not try the plaintext file. Environment tokens still
have first priority, so the boundary wrapper must continue rejecting them.
`HEV_OP_TOKEN_FILE` and its existing default continue to work when DPAPI is
not configured. Keychain and Linux credential stores are left for future
implementations of the credential-storage seam.

For a new store, pipe a token from an approved input source into stdin:

```powershell
# Read-Host's secure input avoids echo/history; no token is a process argument.
$secure = Read-Host 'Service account token' -AsSecureString
$ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try {
  [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) |
    hev secrets token-store create
  if ($LASTEXITCODE -ne 0) { throw 'Token store creation failed' }
} finally {
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  $secure.Dispose()
}
hev secrets token-store inspect
```

Creation makes a new dedicated directory private, refuses an unsafe existing
directory or any existing destination file, and prints only path, scope and
ACL reader SIDs. Do not enable PowerShell transcription for credential entry.
Inspection checks ACLs without decrypting. Use an ASCII service-account token
when piping from Windows PowerShell; PowerShell 7 uses UTF-8 by default.

### Move the wrapper

Previously, after the wrapper's account/ACL checks and environment cleanup,
it decrypted its token and fetched each value separately:

```powershell
# Before: token came from the wrapper's protected store, never ambient fallback.
$env:OP_SERVICE_ACCOUNT_TOKEN = $protectedToken
$env:BUILD_TOKEN = & op.exe read 'op://example/build/token'
$env:PACKAGE_TOKEN = & op.exe read 'op://example/package/token'
$env:REPORT_TOKEN = & op.exe read 'op://example/report/token'
& 'C:\Example\bin\worker.exe' @args
```

After, keep those boundary checks and the DPAPI configuration above. The
wrapper does not decrypt the token itself or call `op`. It makes one
resolution for all three references. Set the host's manager in its kit config
(the DPAPI credential is used when that manager is 1Password):

```toml
[secrets]
manager = "1password"
vault = "example"
field = "token"
```

Keep only references in `C:\Example\config\worker.env`:

```dotenv
BUILD_TOKEN=secret://build
PACKAGE_TOKEN=secret://package
REPORT_TOKEN=secret://report
```

Then launch with the reference file:

```powershell
# Existing boundary checks run first; refuse any ambient service-account token.
if (Test-Path Env:OP_SERVICE_ACCOUNT_TOKEN) { throw 'Ambient token refused' }
# Set the HEV_OP_DPAPI_* settings above in this wrapper's resolver environment.
& 'C:\Example\bin\hev.exe' secrets run --cache-only --clean-env `
  --pass-env SystemRoot --pass-env TEMP --pass-env TMP `
  --env-file 'C:\Example\config\worker.env' `
  --ttl BUILD_TOKEN=1h --ttl PACKAGE_TOKEN=1h --ttl REPORT_TOKEN=1h `
  --explain -- 'C:\Example\bin\worker.exe' @args
exit $LASTEXITCODE
```

Use an absolute child executable path. `--clean-env` starts from an empty
child environment; repeat `--pass-env NAME` only for approved nonsecret
settings the child requires. Credentials and resolver settings are not
implicitly forwarded. Without this flag the existing inherited-environment
behavior remains. Any missing required reference aborts the whole resolution
before starting the child; an ambient value cannot substitute for it.
`--cache-only` also refuses direct reads when the daemon is unavailable;
remove it only if the boundary deliberately permits a direct batched fetch.
Values travel only through IPC and the child's environment. Do not let the
child dump its environment or echo secrets.

### Enable the cache in the capture task and verify

1. Install the reviewed binary at its permanent path on the Windows host.
   Inspect the existing store with the settings above, under the task's own
   Windows identity. Do not recreate an existing matching store.
2. Configure `[secrets]` in the task's selected kit config as above.
   Put the same nonsecret `HEV_OP_DPAPI_*` settings in a task launcher script
   outside the repository, protected by the same private ACL. It must clear
   or reject `OP_SERVICE_ACCOUNT_TOKEN`, set those variables, then execute
   the permanent `hev.exe daemon` with the task's existing config path.
   Keep `HEV_SECRETS_CACHE` unset (or remove a previous `off`). Never put the
   token in the script or the scheduled task.
3. Stop the capture task with `hev daemon stop`. In Task Scheduler, preserve
   the installed `\hev\hevd` task's identity, S4U logon, triggers and settings,
   and change its action to run the launcher using PowerShell
   `-NoProfile -NonInteractive -File C:\Example\bin\start-capture.ps1`.
   The launcher must run `hev.exe daemon` in the foreground, wait for it,
   and propagate its exit code. A later task reinstall may replace this
   custom action: inspect it again after reinstalling.
4. Start with `hev daemon start`. From the boundary wrapper's configured
   environment run `hev secrets ping` and `hev secrets status --no-quota`.
   Verify `daemon: up` and `service-account token (DPAPI protected file)`.
   If only a cache is needed, run `hev.exe daemon --secrets-only` under the
   same launcher settings instead of capture; do not run two endpoints.
5. Once the account's cooldown has expired, launch the worker twice. Do not
   delete a cooldown or retry a failed fetch. Compare `--explain`, status
   and the read log. Use `--no-quota` to avoid a quota-inspection request.

Example observations (illustrative counts, no values):

```text
Before: wrapper launched 10 times; 30 separate op read requests.
hev secrets status --no-quota
  daemon: unavailable
  host read log: 0 real requests (old wrapper bypassed kit's log)

After first launch:
hev secrets: BUILD_TOKEN source=read via=daemon
hev secrets: PACKAGE_TOKEN source=read via=daemon
hev secrets: REPORT_TOKEN source=read via=daemon
2026-11-13T10:00:00Z caller=wrapper kind=inject refs=3 labels=BUILD_TOKEN#12345678,PACKAGE_TOKEN#23456789,REPORT_TOKEN#34567890 outcome=ok provider=op

After ten launches within the one-hour TTL:
hev secrets status --no-quota
  daemon: up since 2026-11-13T09:59:00Z
  cache: 27 hits, 3 misses
  real requests: 1 since start
  1Password credential: service-account token (DPAPI protected file)
  host read log: 1 real requests
hev secrets: BUILD_TOKEN source=cache via=daemon
# No additional inject lines for cache hits.
```

The log is `%USERPROFILE%\.hev\secrets\op-reads.log` unless relocated by
`HEV_SECRETS_READ_LOG`. The legacy wrapper's requests are visible in its own
safe request counts or provider quota, not retroactively in kit's log. Log
labels above use illustrative hashes. Status without `--no-quota` can add a
separate quota check. Restarts empty the cache; shorter TTLs, new references
and TTL expiry cost another batch.

Windows CI exercises DPAPI and ACL checks using fake credentials. Actual
engagement ACLs, existing file encoding, scheduled-task identity/profile,
S4U DPAPI availability, reboot startup, and the engagement's child behavior
still require this on-host verification. No real provider account is needed
by the tests.
