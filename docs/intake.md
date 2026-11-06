# Intake

Intake is built into `hev`. It polls deterministically; no agent chooses work.

```
hev intake --hev-describe
hev intake linear once --config intake.toml
hev intake linear run --config intake.toml
hev intake linear status --config intake.toml
hev intake linear reconcile NAME --admitted --config intake.toml
hev intake linear reconcile NAME --not-admitted --config intake.toml
```

`once` polls once and is supported for scheduled polling; it is exempt from the restart guard. `run` polls immediately and then waits the configured interval
between polls. Interrupt stops it. `status` reads local state without an API key
or network calls. All dates and daily limits use UTC.

```toml
interval = "15m"
mode = "dry-run"
state_dir = "C:/intake/state"
dispatch = ["hev", "dispatch"]
daily_cap = 20
pause_file = "C:/intake/state/PAUSED"

[linear]
api_key_env = "LINEAR_API_KEY"
team = "TEST"
assignee = "synthetic-bot"
allowed_actor_team = "TEST"

[[status]]
name = "Review"
action = "triage"
marker_prefix = "example-triage:v1"
mode = "dry-run"
template = "C:/intake/triage.md"
host = "worker"
concurrency = "read"

[[status]]
name = "Ready"
action = "fix"
marker_prefix = "example-fix:v1"
mode = "off"
template = "C:/intake/fix.md"
host = "worker"
concurrency = "code"

[limits]
code_jobs = 1
linear_min_remaining_pct = 20
```

Validate templates before enabling live rules:

```sh
hev intake linear check --config intake.toml
hev intake linear render --config intake.toml --status Review --ticket TEST-42
hev intake linear render --config intake.toml --status Review \
  --identifier TEST-42 --url https://example.invalid/TEST-42 --title "Review this change"
```

`render` selects the rule using `--status`. With `--ticket`, it makes one
read-only Linear query for identifier, URL, title and current status, using the
configured API-key environment variable and the poller's client and request
budget accounting. With all three offline flags (`--identifier`, `--url`,
`--title`), it uses `--status` as the ticket status and needs no key or network.
Online previews warn on stderr if the selected rule differs from the ticket's
current status. The two input modes cannot be mixed. Output is the validated order, using the
same renderer and checks as live dispatch. Its stable synthetic lease name is
for preview only; live dispatch computes its lease from ticket history.

`check` renders every live or dry-run rule with a template using synthetic
offline ticket values, reports all failing rules, and skips off rules and rules
without templates. If none qualify, it reports `No templated rules to check`.
Both commands leave state untouched, never acquire the state
lock or reserve or dispatch work, and can run alongside `run`. Online request
accounting is in memory only and does not update the running poller's counters.
Exit codes are 0 for valid, 1 for invalid templates (including unsafe control
characters: CR, LF or NUL), and 2 for usage/configuration or network errors. Invalid orders
are never printed.

The sample is synthetic. Keep operational config and templates on the running
host, outside this repository. Unknown keys and invalid modes are rejected.

- `interval`: Go duration of at least `5m`; default `15m`.
- `mode`: default for rules, `dry-run` by default; `off`, `dry-run`, or `live`.
- `state_dir`: required local writable directory; one process per directory.
- `dispatch`: argument array, default `["hev", "dispatch"]`; no shell.
- `daily_cap`: maximum live admissions per UTC day; zero disables admissions.
- `pause_file`: if present, no Linear or dispatch calls occur.
- `linear`: all four fields are required. Team fields are team keys. Assignee
  accepts an exact name/display name or UUID; ambiguous names fail closed.
- `status`: unique status names and slug actions; rule mode overrides global
  mode. `template` and `host` are required for live rules. `concurrency` must be
  `read` or `code`; each action must have one concurrency class.
- `limits.code_jobs`: maximum running code jobs; zero blocks code admissions.
- `limits.linear_min_remaining_pct`: 0–100; below this requests **or** complexity
  remaining percentage, stop the current poll immediately and skip the next poll. Zero disables percentage backoff.

Every poll makes one candidate query with team, assignee and configured statuses,
requesting only issue id, identifier, URL, title, state id/name and updatedAt.
The same query resolves the bot's id. More than 250 candidates fails closed;
narrow the configuration rather than silently losing tickets. Off rules skip
history. History is paginated and cached by issue id/state/updatedAt in local
state; changed issues recheck their latest status entry and latest assignment to
the bot. Both the assigning user and the latest status-transition user must belong to the allowed team and have guest=false, including when membership is refreshed. Membership is refreshed at most one interval after a cached check, including for unchanged candidates. System, integration, guest and outside-team transitions cannot mint work. Missing
history or a system actor fails closed. No descriptions or comments are fetched.

Lease names are lowercase `<identifier>-<action>-<six SHA-256 hex characters>`;
the hash covers issue id, state id and exact status entry timestamp. A later
entry into the same state gets a new lease. `dispatch ls --all --json` checks
all records, including finished jobs; incomplete inventory blocks decisions.
Only names matching this team’s identifier/action/hash lease pattern consume code slots. Inventory names include reservations as well as rows. An append-only admission ledger prevents redispatch after dispatch cleanup. Read admissions can run alongside
code jobs. An uncertain local admission reserves its cap and code slot before
calling dispatch. Each inventory check moves observed Pending leases to the admission
ledger. Pending leases absent for one hour move to `needs-reconcile`, visible
in `status`, and stop consuming code slots while continuing to block that lease.
Legacy reservations without timestamps begin the grace period at first observation.
After inspecting dispatch evidence, an operator runs `reconcile NAME --admitted`
to ledger confirmed work or `reconcile NAME --not-admitted` to allow a retry.
Both retain the original uncertain attempt's daily charge. Reconciliation requires
the state lock and no API key. It is journaled for crash recovery.

Deterministic and local preparation failures are logged and allow later tickets; an uncertain launch stops the poll. Host-full releases its reservation without incrementing ticket failure backoff. That lease
skips 1, 2, 4, 8, 16, 32, then at most 64 eligible polls after consecutive failures;
success clears its backoff. Pending and reconciliation gates still take precedence.
The counters persist across restarts and are visible in `status`.

Dry-run emits JSON decisions and never reads templates, creates order files or
admits work. Required operational bookkeeping still writes `state.json` and a
UTC daily `.jsonl` log: request counts, process starts, cached history metadata,
last poll and today's decisions. Decisions retain seven UTC days; history cache retains only current candidates. Ticket failure and reconciliation entries retain current candidate leases and inventory names; unchecked candidates keep their gates until history is available. State is saved once per poll; a synced append-only reservation journal protects crash recovery between admissions. Logs include identifiers and URLs truncated after the identifier, never title slugs,
API keys or ticket bodies. Protect and retain this local directory accordingly.
Live mode substitutes `{{identifier}}`, `{{url}}`, `{{title}}`, `{{name}}` and
`{{status}}` once in the template. Its flat frontmatter must contain
`name: {{name}}` and a repo, and pass kit’s strict flat-frontmatter parser rules before admission; the configured host is passed with `--host`. The worker should
read the ticket itself. On Unix, temporary order files are created with mode 0600 in state_dir/tmp (directory mode 0700). Windows requires an owner-only ACL on state_dir; Go file modes do not set Windows ACLs. Files inherit that ACL. They are
removed after dispatch succeeds or fails. The next writer startup removes abandoned
order files under the state lock. Template validation logs omit rendered values. The API key is removed from the
dispatch subprocess environment case-insensitively, including Windows. Dispatch stderr is captured in memory; exit status 1 with an exact known parser/validation rejection or narrowly matched host-full message releases its cap reservation and are logged as failed. Unknown errors remain uncertain.

```markdown
---
name: {{name}}
host: worker
repo: example-repository
base: main
---
Read {{identifier}} at {{url}} and perform the configured review.

## Acceptance
- Record findings on the issue.
```

Rate-limit headers are checked on every response. A depleted quota waits for its
millisecond Unix reset time; HTTP 400 (or any response) with GraphQL `RATELIMITED`, and HTTP 429, honor quota reset and `Retry-After` (seconds or
HTTP date), defaulting to one hour if neither provides a future deadline. No
request retries occur within a poll. Transport and 5xx failures back off exponentially across polls, from five minutes to one hour. Errors are logged without GraphQL response
bodies. Counters count attempts, including rejected requests. Admission attempts
count toward the daily cap even when their outcome is uncertain.

The fourth `run` start in a rolling hour exits with a restart-guard error; refused starts are not recorded. `once` does not count starts. An OS lock on `RUNNING` prevents concurrent writers and releases automatically on process death, so a stale file needs no removal. Windows state replacement retries sharing violations with bounded backoff.
For an uncertain admission inspect its named dispatch record before using the reconcile command. Preserve the admission and reservation ledgers and daily counts; changing `Pending` alone does not erase journaled reservations. Never delete state merely to bypass these guards.

## Windows launcher

First protect a fresh state directory under the account that runs the task:

```powershell
$ErrorActionPreference = 'Stop'
$stateDir = 'C:/intake/state'
if (Test-Path $stateDir) { throw 'Use a fresh state_dir or audit its ACL first' }
New-Item -ItemType Directory -Path $stateDir | Out-Null
$owner = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls $stateDir /inheritance:r /grant:r "${owner}:(OI)(CI)F"
if ($LASTEXITCODE -ne 0) { throw 'Unable to restrict intake state ACL' }
& icacls $stateDir
```

Verify that only the task owner has access. Existing directories require an ACL
audit, including explicit grants on all children. The ACL must permit the same
account used by the scheduled task.

PowerShell 7 resolves the secret **once**, passes it in the child's environment
and waits for the long-lived process. Configure the reference on the host;
do not put a secret value in scripts, task arguments or logs.

```powershell
$ErrorActionPreference = 'Stop'
# INTAKE_KEY_REF is a host-local secret reference, not a secret value.
$key = & op read $env:INTAKE_KEY_REF
if ($LASTEXITCODE -ne 0 -or -not $key) { throw 'Unable to resolve intake key' }
$start = [System.Diagnostics.ProcessStartInfo]::new()
$start.FileName = 'C:/tools/hev-intake.exe'
$start.UseShellExecute = $false
foreach ($arg in @('linear', 'run', '--config', 'C:/intake/intake.toml')) {
  $start.ArgumentList.Add($arg)
}
$start.Environment['LINEAR_API_KEY'] = $key.Trim()
$key = $null
$child = [System.Diagnostics.Process]::Start($start)
$start.Environment.Remove('LINEAR_API_KEY') | Out-Null
$child.WaitForExit()
exit $child.ExitCode
```

Use a scheduled task that starts this once and has automatic restart disabled.
The process restart guard cannot protect the launcher's earlier secret lookup;
a launcher retry loop would still spend secret-store quota. Enable live modes
only after reviewing dry-run decisions and obtaining the operator's go.

## Batch rules

Set `batch = true` on a status rule to collect its eligible tickets into one
order per poll. The default is false. Candidates use the same assignee,
human authorization, membership and ticket failure backoff checks as individual
rules. A denied ticket gets its own skipped decision and does not block the
others. Each included ticket gets an `included` decision; a separate batch
decision contains the sorted `keys` and dispatch name. Pause, Linear quota,
daily admission cap and code concurrency limits still apply. A batch charges
one admission. It makes no additional Linear calls to collect the tickets.

```toml
[[status]]
name = "Release ready"
action = "release"
mode = "dry-run"
batch = true
batch_cooldown = "6h"
template = "C:/intake/release.md"
host = "worker"
concurrency = "code"
```

`batch_cooldown` is a nonnegative Go duration on the rule, default `6h`.
An active batch blocks further batches for that rule, including changed sets.
Pending or unreconciled admissions also block it. After the run ends, a changed
eligible set can start on a later poll; the exact same set waits until its
cooldown since admission expires. This state survives restarts. Names use
`<action>-<yyyymmdd>-<six SHA-256 hex characters of sorted keys>`, with UTC dates.
The effective cooldown for the same set is the later of `batch_cooldown` and
the next UTC day. If the computed name is already in dispatch inventory or the
admission ledger, intake skips it with `dispatch record exists`, without charging
the daily cap, adding failure backoff, or restarting the cooldown.

Batch templates accept `{{identifiers}}` (sorted keys, one per line), `{{urls}}`
(corresponding URLs, one per line), `{{count}}`, `{{name}}` and `{{status}}`.
`check` rejects `{{identifier}}`, `{{url}}` and `{{title}}` in batch templates,
and rejects the three batch placeholders in single-ticket templates. Individual
values receive the same CR/LF/NUL validation before joining lines.

```markdown
---
name: {{name}}
host: worker
repo: example-repository
base: main
---
Build one release PR from these {{count}} {{status}} tickets:
{{identifiers}}

Read each ticket at its URL and cherry-pick its release commit:
{{urls}}

## Acceptance
- Report the tickets included and any tickets left out, with reasons.
- Open one release PR containing the selected commits.
```

Offline batch rendering accepts repeated `--identifier` / `--url` pairs in
corresponding order; `--title` is optional. Online rendering accepts repeated
`--ticket` keys and makes one read-only query per ticket. Both render the whole
batch using the selected rule's status. Single-ticket rules require one ticket.

```sh
hev intake linear render --config intake.toml --status 'Release ready' \
  --identifier TEST-1 --url https://example.invalid/TEST-1 \
  --identifier TEST-2 --url https://example.invalid/TEST-2
hev intake linear render --config intake.toml --status 'Release ready' \
  --ticket TEST-1 --ticket TEST-2
```

## Stopped-run comments and retry

Set `marker_prefix` on each status rule to the exact marker prefix its worker
uses in Linear comments (for example, `example-fix:v1` for that fix template).
Release and triage rules use their own template's prefixes. An empty prefix
keeps legacy rules compatible but disables completion comments for that rule;
configure it before dispatching runs that need this protection.

Before dispatch, intake saves the run name, dispatch time, rule and tickets in
its existing state. Once inventory reports `done`, `failed` or `stopped`, it
checks every ticket, including each ticket in a release batch. A comment by the
bot viewer containing the rule's prefix and posted after dispatch counts as a
worker update. Otherwise, while the ticket is still assigned to the bot, intake
posts:

```text
hev-intake:v1 phase=stopped run=<run name>
hevbot's <action> run stopped without updating this ticket (exit <code>).
Nothing was changed. To retry, move the ticket out of <status> and back.
```

If inventory has no exit code, `<code>` is `unknown`. This fixed notice contains
no worker reports, diagnostics, hostnames or credentials. It records a missing
ticket update; intake does not inspect or roll back the worker's changes.

Comment outcomes persist per run and ticket in `state.json` and appear under
`comment_runs` in `status`. Intake also checks for the exact stopped-comment
header before posting, so a crash after Linear accepts the comment but before
local state is saved does not duplicate it. Reassigned tickets are skipped.
Dry-run rules log `would-comment`; off rules do not post. Linear failures retain
a retryable `failed` outcome with persisted per-run, per-ticket attempts and next-retry time.
Retries start at five minutes, double up to one hour, and survive restarts.
Transport errors, HTTP 5xx and rejected comments delay only that ticket; they
do not suppress dispatch in the same poll or pause subsequent polling. Actual
Linear rate limits (HTTP 429 / RATELIMITED) and quota thresholds still use the
shared backoff, as other Linear requests do. Failures are retried without
waiting inside the polling loop. Existing dispatches made before run tracking
was enabled are not reconstructed from ticket contents.

To retry, a human moves the ticket out of the triggering status and back into
it. The new status-entry time produces a new lease/run name; the old dispatch
record remains intact. The existing human authorization checks still apply.

## Comment mentions: feedback on the existing branch

A `[[mention]]` source consumes the configured bot key's own Linear
`issueCommentMention` notifications. The key must belong to the configured
assignee. Only comments on the configured Linear team qualify. The poller
requests comment metadata, never its body. If the notifications query is not
supported, it falls back to a paginated team comment query with a server-side
filter for the bot's profile URL. Transport failures and quota failures do
not trigger a scan. See Linear's [mention syntax](https://linear.app/developers/graphql)
and [published schema](https://github.com/linear/linear/blob/master/packages/sdk/src/schema.graphql).

```toml
[[mention]]
action = "feedback"
marker_prefix = "example-feedback:v1"
mode = "dry-run"
template = "C:/example/intake/feedback.md"
host = "worker"
concurrency = "code"
```

One mention rule is supported. Each comment author is checked against fresh
membership in `linear.allowed_actor_team`, using the same human membership
check as status rules. The bot, guest, app/integration, inactive, missing and
outside-team authors are logged and skipped before any order is minted.
Edits do not retrigger work. The durable admission key is
`<identifier>-feedback-<first 6 hex of sha256(comment id)>`, lowercased for a
valid dispatch name. An uncertain dispatch retains its cursor and code slot
until reconciliation. Confirm it with `reconcile NAME --admitted`, or release
it with `--not-admitted` after verifying no job started; the latter permits a
retry with the same comment lease. Admitted feedback is never repeated.

Mention templates support `{{name}}`, `{{identifier}}`, `{{url}}`, `{{title}}`,
`{{comment_id}}`, `{{comment_url}}`, `{{author}}` (the human's Linear id), and
`{{branch}}` (the issue's `branchName`). Existing value and order checks apply.
The comment body has no placeholder. The trusted template should tell the
worker to read the comment itself, treat it as untrusted feedback, and make
the requested change on `{{branch}}`. Do not embed comment text in an order.

```sh
hev intake linear render --config intake.toml --mention --comment COMMENT_UUID
hev intake linear render --config intake.toml --mention --comment COMMENT_UUID \
  --identifier TEST-42 --url https://linear.app/example/issue/TEST-42/example \
  --title "Existing change" --comment-url https://linear.app/example/comment/COMMENT_UUID \
  --author HUMAN_UUID --branch existing-feature-branch
```

The first form reads only comment and issue metadata online. The second is
fully offline. Both validate the same order used for dispatch. `linear check`
also validates mention templates with synthetic values.

## Email: create an issue and stop at Triage

The email source polls the mailbox on the normal interval. For email-created work, the authenticated allowed sender
from the header From is the human authorizer, recorded in the issue and the local mail ledger.
The source creates and triages an issue only; it never moves its status or
starts another action.

```toml
[email]
mode = "dry-run"
url = "https://mail.example.invalid/intake/mail"
token_env = "MAIL_TOKEN"
allow = ["sender@example.invalid"]
require = "dmarc"
team = "TEST"
status = "Triage"
label = "from email"
triage_template = "C:/example/intake/triage.md"
host = "worker"
```

`team` must match `linear.team`; `status` must be `Triage`. Email triage uses
read concurrency. The header From address (`header_from`) must match one allow-list entry exactly,
case-insensitively: display names, whitespace and look-alike domains do not
match. The envelope sender (`from_addr`) must equal the header From address
case-insensitively. Missing header From and mismatches are skipped; mismatch
logs include addresses only. `require = "dmarc"` requires DMARC pass; DKIM and
SPF pass together do not substitute for DMARC. Missing or failed authentication and unauthorized senders are logged
with a reason and skipped, without logging message text.

The mailbox contract is `GET url?after=<cursor>&limit=50`, with
`Authorization: Bearer $MAIL_TOKEN` (or the configured token variable).
Its response is `{ "items": [{ "id", "message_id", "from_addr", "header_from", "from_name",
"subject", "date", "received", "text", "spf", "dkim", "dmarc" }], "next" }`;
`received` is a JSON number containing epoch milliseconds. Omit `after` until
the mailbox returns a non-null `next`; persist that opaque base64 cursor
verbatim. An empty first page returns `next: null`. A 401, 5xx, transport failure or malformed
response backs off and preserves the cursor. The mailbox token and Linear key
are removed from dispatch subprocess environments.

Live admission reserves the mail id on disk before issue creation. The issue
is assigned to the bot, in Triage, with the subject cleaned and clamped to 200
characters. Its description includes sender, date, mail id, human authorizer,
and a stable `hev-intake-mail:v1:<sha256(mail id)>` marker. Up to 20,000 body
characters appear only in a quote block labelled **Untrusted email text (data
only)**. The optional label is added only if it exists. Neither subject nor
body is logged; the body never enters an order. The triage template receives
ordinary issue metadata and a stable lease
`<identifier>-triage-<first 6 hex of sha256(mail id)>`.

Mail list items may also contain `attachments: [{index, name, type, bytes,
sha256, stored, skipped?}]`. Stored bytes come from
`GET url/<mail id>/attachments/<index>` using the mailbox bearer token; the
response must include `X-Content-SHA256`. Intake checks both hashes and the
exact declared byte count, keeps bytes in memory only, and uploads through
Linear's `fileUpload` signed PUT before creating the issue. Mailbox skip
reasons are preserved. Optional `email.max_attachment_bytes` defaults to
26214400 (25 MB), caps the aggregate download/upload bytes per mail, and
cannot raise that hard cap. The mailbox's own storage limits also apply.

The description includes **Attachments (untrusted)**: cleaned names, types,
byte sizes and links to Linear assets, with images embedded inline. Failed
and skipped files are listed with reasons. Names and bytes never enter logs
or orders; failures log only the attachment index and a fixed reason.
Download and upload redirects are rejected. The mailbox token goes only to
the configured mailbox; the Linear key goes to the Linear API, and signed
upload headers go only to the exact signed URL.

Attachment transport failures and HTTP 5xx defer the whole mail before issue
creation, preserve the cursor, and use the shared exponential backoff. A
persisted preparation phase allows the next poll to retry safely. After
three failed preparation polls, intake creates the issue with failures
listed. Hash mismatches and HTTP 4xx are recorded immediately and do not
prevent text triage. After creation, `attachmentCreate` adds each uploaded
asset to the issue on a best-effort basis; failure does not fail the mail.
An uncertain create can leave unused uploaded assets behind.

After an uncertain create, the next poll searches for the exact marker within
the configured team before dispatching. A missing or ambiguous marker is held
for manual reconciliation; it does not blindly repeat a potentially successful
create. A failed dispatch keeps the existing issue and records
`needs-reconcile`. Inspect the issue and named dispatch record before resolving
the lease with the existing `reconcile` command. Mail records that need
reconciliation remain held for operator review; resolving a lease does not
silently retry the email. The issue-id ledger suppresses duplicate Triage status
admission and logs that suppression once. Other status rules keep their normal
human authorization requirements.

```sh
hev intake linear check --config intake.toml
hev intake linear render --config intake.toml --email --mail-id MAIL_ID \
  --identifier TEST-42 --url https://linear.app/example/issue/TEST-42/example \
  --title "Email subject"
```

Email rendering is an offline preview of the triage order for the supplied
issue and mail id. It performs no mailbox reads, Linear writes or dispatches.
`check` validates both source templates. Dry-run polling records what would
be created/dispatched and makes no Linear writes or dispatches.

Both sources record and persist the first poll's current time even when
paused, and ignore older comments/mail. The email start time is separate
from its opaque mailbox cursor. Skipped
and dry-run items are consumed; changing to live does not replay them. Pause,
the daily dispatch cap, the shared Linear request budget and backoff apply to
both sources. Feedback consumes `limits.code_jobs`, including active feedback
leases found in inventory. A capped item retains its cursor for a later poll.
`linear status` includes `sources` with both cursors, per-source admitted,
skipped and needs-reconcile counts, and the shared `last_poll` timestamp.
The mail ledger stores ids, authorizer and resulting issue metadata, never
message bodies.

## Service

Install with `hev intake install --config /absolute/path/intake.toml`, then use
`hev intake start|stop|status`. The service uses systemd user units on Linux,
launchd on macOS, and a scheduled task on Windows. Install does not start polling.
Supply credentials in the service environment. Automatic restart is disabled;
the existing persisted restart guard also applies to every `linear run`.

## Integration hooks

The dispatch command remains configurable, including a wrapper for queue admission
or `hev secrets` environment injection. Those commands are not required by intake.
The dispatch order parser on this base does not yet accept `requested_by`; authorizer
identity remains in intake state pending that integration.
