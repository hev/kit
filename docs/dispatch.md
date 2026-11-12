# Dispatch agent work

`hev dispatch` sends an order to a configured machine and leaves a durable run
record under `~/dispatch/<name>/`. Dispatch works without a daemon. Trace
capture still requires the normal capture/indexing setup.

Create `~/.config/hev/dispatch.toml` on the machine sending work. Configure `notify` separately on each target;
the supervisor reads its target configuration again at completion:

```toml
cap = 2 # capacity of this receiving host
workspace = "~/workspace"
notify = ["notify-command", "{reply}", "dispatch {name}: {state} {summary}"]

[validation]
host = "mac"
harness = "claude"
model = "your-validator-model"

[hosts.buildbox]
ssh = "buildbox"
cap = 8 # optional sender ceiling; configure capacity on buildbox itself
namespace = "my-traces"

[hosts.mac]
ssh = "mac"
cap = 2
path_prefix = ["~/.local/bin", "/opt/homebrew/bin"]

[hosts.winbox]
workspace = "C:/src"
harness_wrapper = ["pwsh.exe", "-NoProfile", "-File", "C:/tools/with-creds.ps1"]
ssh = "user@winbox"
cap = 3 # optional sender ceiling
os = "windows"

[hosts.here]
local = true
cap = 2
```

Each remote host needs `hev` and the selected harness on its PATH, Git, and an
existing clone with an `origin` remote. Unix hosts also need tmux. Windows
uses a detached Go process, without tmux or generated PowerShell files.
`hosts` reports the installed version and flags mismatches. SSH uses batch
mode, short connection timeouts, and strict host-key checking. Configure
trusted host keys outside dispatch. `path_prefix` adds executable directories
to remote Unix PATH. Use `local = true` to run on the calling machine.

An order is Markdown with strict flat frontmatter:

```markdown
---
name: improve-help
host: buildbox
repo: kit
base: main
harness: codex
model: your-model
validation: browser
preview: python3 -m http.server {port}
reply: your-notification-destination
issue: TASK-123
class: code
timeout: 120
---
Improve the help text.

## Acceptance
- Help explains the new command and tests pass.
```

`name` must match `^[a-z0-9][a-z0-9-]*$`; `repo` is a directory beneath `workspace`, an
absolute path, or a `~/` path. For relative repositories, dispatch uses the sender's selected `[hosts.<name>].workspace` override, then the target configuration's top-level `workspace`, then `~/workspace`. The sender forwards only explicit per-host workspace overrides. An empty workspace in an RPC lets the target choose its own default. Absolute Unix, Windows drive, and UNC paths take precedence. `base`, `harness`, `validation`, and `timeout`
default to `main`, `codex`, `none`, and 120 minutes. `claude` is the other
harness; `browser` is the other validation value. `model`, `reply`,
`issue`, `class`, `context`, and `preview` are optional; `linear` is accepted as an alias for `issue`.
`context` names a context recipe in the repository; dispatch builds its bundle
at launch and gives it to the worker (see [Context recipes](#context-recipes)).
`class` names the order's concurrency class with the same pattern as `name`; see
[Dispatch classes](#dispatch-classes). [`schemas/order.schema.json`](../schemas/order.schema.json)
lists every frontmatter key. Unknown,
duplicate, empty, nested, and malformed fields are rejected. Every order
must contain `## Acceptance`. `--host` overrides its host selection.

```sh
hev dispatch order.md
hev dispatch hosts --json
hev dispatch ls --all
hev dispatch show improve-help --json
hev dispatch logs improve-help -f
hev dispatch stop improve-help
hev dispatch rework improve-help 'Also test the error case'
hev dispatch clean improve-help
```

`--config PATH` or `HEV_DISPATCH_CONFIG` selects an alternate configuration. A host can set `config_path` to select its target-local configuration for supervisors; this is useful for isolated smoke tests. Paths beginning with `~/` expand on the target. `ls`, `show`, and `hosts`
support `--json`. Listings inspect hosts in parallel; an unreachable host
occupies one line. Without `--all`, `ls` shows running and unexpectedly dead
runs. Names are searched across hosts; use `--host` if a name is ambiguous.

Admission reserves a live PID-checked slot under a short target lock, then
releases the lock before fetching and creating the worktree. A full host fails with `host full (n/cap)`
and other hosts' free slots; there is no queue. Existing names are refused,
after normal worktree cleanup. Failed admissions can be removed entirely with `clean`, allowing name reuse. Dispatch fetches `origin` and creates branch `d/<name>`
at `origin/<base>` in `~/dispatch/<name>/wt`. Setup failures record a failed result and release the reservation. SSH mutations
allow five minutes for the two bounded Git operations. A lost connection reports
an unknown outcome: inspect the named record before retrying. Requests travel on
stdin so large order files do not exceed command-line limits. `clean` uses Git's normal worktree removal checks and keeps
the order, status, report, logs, metadata, and result for admitted runs; it does not delete the
branch or remote PR for admitted runs. For failed admissions, cleanup also removes an already-created implementation branch when Git considers it merged; an unsafe branch removal leaves the record and reports the error.

The supervisor owns timeout and completion independently of the agent. On
Unix it runs in tmux session `d-<name>`, which can be attached directly. Live
tmux sessions determine occupancy. Windows checks supervisor PID and process
creation time to avoid counting reused or stale PIDs. Termination signals (interrupt, hangup, termination) kill the harness tree and
write a failed result with the signal recorded. A stop request kills
the harness process tree and records `failed` with exit code 143. `stop` also uses the recorded harness process group when the supervisor has
died unexpectedly. A timeout
records `timeout`. Other exits become `done` or `failed`; a missing supervisor
and result becomes `died`. Rework requires a stopped run and reuses its
worktree and branch; it archives previous attempt logs, status, report and
result before appending a `## Rework` note.

The fixed worker prompt requests status updates, commits, branch push, a draft
PR, and a report. Explicit order instructions override those defaults, so a
scratch smoke order can prohibit pushes and PRs. Harness output goes to `log`;
Codex thread IDs and Claude session IDs are saved in `result.json` as soon as
they appear. The final result includes timestamps, exit code, branch, HEAD,
and a PR URL when `gh pr view` finds one. Notifications execute the configured
argument array with placeholder substitution, without a shell. Failure is
recorded as `notify_error` and cannot change the run state. Browser validation runs as a linked run on the configured validation host, and its verdict is included in notifications.
Runs started by invited teammates skip `{reply}` templates; the host keeps
their notices for `hev dispatch notices pull` instead (see
[Share a dispatch host](nodes.md#how-run-notices-arrive)).

## Run outcomes and decisions

The worker prompt asks a one-shot implementation run to make the last line of
`../status.md` exactly one final marker:

- `done`
- `blocked: <reason>`
- `needs-decision <KEY>: <question>`, where KEY is a short word such as `db`
  (letters, digits, `.`, `_`, `-`; at most 40 characters)

The supervisor parses that line into `result.json` as `outcome`, with `key`
and `question` for a decision or `reason` for a block. A marker refines only a
clean exit, using the existing states:

| Marker | State |
| --- | --- |
| `done` | `done` |
| `blocked: ...` | `blocked` |
| `needs-decision KEY: ...` | `needs-review` |
| none or malformed | `done` or `failed` by exit code, as before |

A non-zero exit, timeout or stop keeps `failed` or `timeout` whatever the
marker says. `needs-review` already means a person must act; the outcome
fields say why. Validators and repeating rounds get no marker instruction.
`blocked` and open decisions are shown by `ls` without `--all`, and its
`DECISION` column shows `KEY: question` or the block reason. The notify
`{summary}` starts with the marker, for example
`needs-decision db: Postgres or SQLite?; exit=0`. `show --json` returns
`result.json` and `decisions.json`.

Answer an open decision by key:

```sh
hev dispatch say pick-db --resolve db 'SQLite'
```

`--resolve` records the answer in the run's `decisions.json` ledger and
starts a rework on the same worktree, branch and PR. The `## Rework` note
names the key, question and answer. Every later prompt for the run repeats
the resolved decisions, and each result lists them under `decisions`. A key
that is not the run's open decision is refused. So is an already-resolved key,
a run that is still live, and a resolve from a teammate key. Plain `say` on an
ended run still reworks with a free-form note and leaves the decision
unanswered.

A host running an older `hev` writes no outcome, so its runs behave as before.
The sending CLI checks the target's `show` for the open key before it sends a
resolve. An older host therefore refuses the resolve instead of ignoring the
field and starting an unrecorded rework.

During indexing, sessions and searchable chunks whose working directory is
under `~/dispatch/<name>/` receive `order=<name>`. The order file supplies an
optional `issue` value. This requires indexing with a version that supports
these attributes; older indexed traces need reindexing.

```sh
hev traces ls --order improve-help
hev traces search 'why did it fail' --order improve-help
```

A host's optional `namespace` is passed as `HEVLAYER_NAMESPACE` in the worker's
environment. Configure the capture daemon's destination consistently with
that namespace. Dispatch does not start, restart, or reconfigure capture.

Unix launchers forward a defined environment allowlist (including harness API
keys and authentication paths) through tmux rather than inheriting its stale
server environment. Set `env = ["CUSTOM_KEY"]` inside `[hosts.<name>]` to extend the list of names to forward. The old `[hosts.<name>.env]` table is rejected; values come only from the admitting process environment. Metadata stores names only, never environment values. Unix forwards exactly those names with `tmux new-session -e NAME=VALUE`. Names an applied orchestrator declares in `source.env` are plugin-only: they are never forwarded, are unset before the worker supervisor starts, and are removed from the worker harness, round harness, notification and helper-command (Git, `gh`, SSH, tmux) environments, even when `env` lists them. Windows does not forward credentials across scheduled tasks; Windows hosts must fetch credentials through `harness_wrapper`, for example from a vault. `logs -f` transfers
only new bytes from a tracked offset. `traces ls --order` searches all ages
unless a time window is explicitly selected.

Windows launches with job breakaway; if SSH disallows breakaway, it uses a
one-shot scheduled task. The harness starts suspended, joins a kill-on-close
job, and only then resumes, retaining
control of descendants even when the root harness exits. Its admission lock is
a temporary directory; if a dispatcher crashes during admission, inspect the
record and determine whether an admission operation is still active before
recovering a stale lock.

## Context recipes

An order with `context: PATH` names a JSON recipe in its repository, read at
the run's base commit. On the target, after the worktree exists and before the
harness starts, every launch (start, rework, retry, ended-run `say`, amend
restart, host move) builds the recipe's bundle from the worktree into
`~/dispatch/<name>/contexts/<n>/`, with the order body as its assignment. The
harness receives the bundle's `context.md` on standard input; the prompt
argument is unchanged. A missing or invalid recipe, or a missing source, fails
the attempt before launch with an error naming it. `meta.json` records the
digest, recipe path, recipe hash and commit, and the source manifest;
`show` prints them, `show --json` and `ls --json` include `context_digest`, and
indexed worker sessions carry `context_digest` for `hev traces ls --context`.
Older targets refuse orders with `context:`. The bundle is not everything the
harness saw: it still loads `AGENTS.md`, `CLAUDE.md`, skills and user
configuration on its own. See [Composable context](context.md).

## Browser validation

Only orders with `validation: browser` launch browser validation. After a
successful implementation run, the supervisor uses `[validation]` to launch a
validator on a separate host. Other work needs only its receipt and CI.
Configure dispatch on the implementer's host as well as the calling host. A
locally selected implementer host also needs an `ssh` address and any
`path_prefix` needed for validator callbacks. Windows validators are unsupported.

```sh
hev dispatch validate improve-help
hev dispatch validate improve-help --host mac
hev dispatch validate improve-help --allow-same-model
hev dispatch rework improve-help --from-validation
```

Manual validation requires a completed browser order and a branch whose pushed
head matches the recorded SHA. The validation harness and model must differ as
a pair from the implementer's; identical pairs are refused with both identities
in the error, unless `--allow-same-model` is supplied. This override does not
permit validation on the implementer's host.

The validator record is `<name>-v<N>`. Dispatch fetches origin in the validation
host's existing clone and creates a detached worktree at the recorded SHA.
The original order and a fixed validator prompt instruct the validator to
check each Acceptance item in a real browser, save screenshots under
`../evidence/`, and write `../report.md` and `../verdict.json`. Validators must
never edit implementation code, commit, push, or create a PR. `preview` can be
a URL or a command run in that worktree; `{port}` is replaced with an available
local port. Quote preview scalars containing a literal `#`.

```json
{"verdict":"pass","items":[{"item":"Clicking the button shows a message.","result":"pass","evidence":["../evidence/click.png"],"note":"Message appeared after clicking."}]}
```

Verdicts require nonempty items, pass/fail results, observations, and evidence
for passing items. The aggregate verdict must agree with the items. Missing
or malformed verdicts fail validation. The supervisor writes `result.json`,
returns the verdict to the parent, and notifies through the configured command.
`ls` includes a validation column; `show` includes `validation.json`, with the
latest verdict and validation-host evidence paths. Validator sessions carry
the parent's order tag, so `hev traces ls --order NAME` includes both kinds of
session after indexing.

A full validation host leaves the parent's validation state `pending` and
notifies; there is no queue. Retry explicitly with `validate NAME` once capacity
is available. `running`, `pass`, and `fail` are the other validation states.
After a failed verdict, `rework --from-validation` appends the verdict items to
the original order and relaunches the implementer. Its next successful finish
launches validation again. If the same Acceptance item fails twice, the parent
becomes `needs-review`, notification says so, and further validation-driven
rework is refused. An operator can supply an explicit rework note to reset the failure streak and resume automatic validation.

Validation records include the tested head SHA. Late verdicts for an older head
are retained as `stale` and cannot change the current result. Reservations have
an expiry so a validator that dies without a callback cannot block retries
forever. Expired reservations are failed when validation is retried.

A plain `rework NAME NOTE...` starts a fresh failure streak. Only
`rework --from-validation` continues the existing streak. The default `ls`
includes orders needing review and validation pending, running, or failed.
Validator callbacks resolve the parent host name using the validator's own
configuration; configure that route on the validation host. Notifications run
in a separate process after the verdict is committed; failures appear in
`notify_error` without failing the callback. For isolated local smoke tests,
both configured hosts may explicitly use `local = true`.

Host names must be consistent across every machine's dispatch.toml: the
validator looks up the parent's host name in its own config. If that entry is
missing, validation fails immediately; the validator retains a terminal failure
in result.json, visible in ls and show, naming the missing host entry.

Validation requires a separate host, including when both routes use local=true.
For local testing only, set validation.allow_same_host = true explicitly.
These runs carry same_host=true in result.json and a same-host label in ls.

## Inline tasks and messages

Create an order without a file:

```sh
hev dispatch run sample 'Improve the help text' --accept 'Help includes examples'
printf 'Improve the help text\n' | hev dispatch run sample -
hev dispatch say improve-help 'Also cover empty input'
hev dispatch attach improve-help
```

`run` accepts `--host`, `--harness`, `--model`, `--base`, `--name`,
`--timeout` (minutes), `--reply`, `--issue`, and repeatable `--accept`.
Its generated Markdown is saved as the run's order. The default name is a
task slug plus date, with a numeric suffix when needed. Without `--accept`,
acceptance is “The task above is done as described”. Set `default_host` at
the top level of the configuration to prefer a host; otherwise `run` picks
the reachable host with most free slots (ties use alphabetical order).
Admission still checks capacity and reserves the name atomically.

`say NAME MESSAGE...` appends a timestamped entry to a live run's
`inbox.md`. A message of `-` reads standard input. Workers are instructed
to check the inbox between steps; delivery does not interrupt an active step.
For an ended run, `say` starts rework with the message as its note;
`say NAME --resolve KEY ANSWER` answers an open `needs-decision` instead.
`show` includes the inbox. On Unix, `attach` attaches to the local tmux
session or connects through SSH; on Windows it prints the log path and a
`logs -f` hint.

`default_host` is a preference: if it is full or unreachable, selection falls
back to the reachable host with most free slots. `run` prints the selected
host. Inventory includes reserved directories without metadata, and explicit
names are checked against known records on every host. Unreachable hosts leave
name availability unknown; `run` prints a warning before proceeding, and target
admission still refuses an existing name atomically.

Rework archives `inbox.md` with the other attempt files so previous messages
are not replayed. A message sent after completion becomes a Rework note.
`attach` uses the target host's OS. For a local Unix target inside tmux it
switches the current client; remote Unix attachment honors `path_prefix`.

A test that fails and then passes on rerun is a flaky test, not a pass. Workers must report it by name in `report.md`, fix it when in scope, and never rerun until green.

A host's `harness_wrapper` is an argv list prepended to the harness executable
and its arguments after a `--` sentinel for both workers and validators. Admission resolves it
from the target's own `[hosts.<name>]` entry; the target wrapper always applies. A sender entry supplies a wrapper only when the target has none; an empty sender list cannot disable the target wrapper. Arguments retain their boundaries
through the operating system's process launcher, including Windows quoting.
The wrapper must wait for the harness and exit with the harness's exit code;
on Unix a wrapper script can use `shift; exec "$@"`. Do not put credentials in wrapper
arguments. Admission failures retain a terminal `failed` result with a reason,
visible through `ls --all` and `show`; `clean NAME` removes these failed-admission
records entirely, while preserving the usual cleanup behavior for admitted runs.

The host keys `workspace`, `harness_wrapper`, and `config_path` require a version
supporting host launch settings on every binary that reads that configuration.
Older binaries reject these keys; upgrade before adding them. Relative
`config_path` values resolve beside the configuration that names them.
Admission validates the selected target host; invalid unrelated routes produce warnings.
An uncertain Windows launch is `launch-unconfirmed`, and clean refuses it.
After 30 seconds, ls/show or clean can mark it failed if its supervisor is absent.
This remains an admitted record, so normal clean preserves its receipt.

Wrappers must consume `--`, pass every subsequent argument verbatim, wait, and
return the child exit code. Use PowerShell 7 with ProcessStartInfo.ArgumentList
(no cmd.exe or reconstructed command strings):

```powershell
# with-creds.ps1: configure the scoped child environment here.
$argv = @($args)
if ($argv[0] -ne '--' -or $argv.Count -lt 2) { exit 2 }
$start = [System.Diagnostics.ProcessStartInfo]::new()
$start.UseShellExecute = $false
$exe = (Get-Command $argv[1] -CommandType Application -ErrorAction Stop |
  Select-Object -First 1).Source
$rest = @($argv | Select-Object -Skip 2)
if ($exe -match '\.(cmd|bat)$') {
  # npm shims are batch files; running them goes through cmd.exe, which
  # reparses arguments. Run node.exe on the shim's JavaScript entry point.
  $js = Join-Path (Split-Path $exe) "node_modules/@openai/codex/bin/codex.js"
  if (-not (Test-Path $js)) { Write-Error "no JS entry point next to $exe"; exit 2 }
  $rest = @($js) + $rest
  $exe = (Get-Command node.exe -CommandType Application -ErrorAction Stop |
    Select-Object -First 1).Source
}
$start.FileName = $exe
foreach ($a in $rest) { $start.ArgumentList.Add($a) }
$child = [System.Diagnostics.Process]::Start($start)
$child.WaitForExit()
exit $child.ExitCode
```

Use `pwsh.exe -NoProfile -File C:/tools/with-creds.ps1` as the wrapper argv.
The sentinel contract is breaking for existing Unix `exec "$@"` wrappers: change
them to `shift; exec "$@"`. A Windows-only regression test checks that PowerShell
7 `-File` preserves the literal `--` in `$args`.
Windows PowerShell 5's native argument reparsing is not a supported pass-through
wrapper; use a native executable launcher or PowerShell 7.

## Target capacity

Starting in 0.6, the receiving host owns capacity. Set top-level `cap = N`
in its default dispatch configuration (before any TOML table). When omitted,
the target uses its own legacy host entry's cap (the entry matching its
hostname, or a single `local = true` entry), with a note to move it to top-level
`cap`. If neither supplies a cap, it defaults to 2. Explicit top-level values
must be positive. Precedence is top-level `cap`, then own host entry, then 2.

Upgrade every binary reading a configuration before adding top-level `cap`:
every target binary must support this setting before it is added. Older
binaries reject the new key, so retain the legacy host cap until that upgrade
is complete; existing capacity continues to apply during the transition.

A sender's
`hosts.NAME.cap` can lower the cap for its requests, never raise it. Each
request compares the total live runs and reservations against the lower limit;
this is not a separate per-sender quota. The existing admission lock protects
both the check and reservation. `hev dispatch hosts` reports the target cap
and target free slots, even when a sender has a lower limit.

## Dispatch classes

A target can also cap concurrent runs per order `class` with a top-level
`classes` table in its default dispatch configuration (before any TOML table):

```toml
cap = 4
classes = {code = 1, docs = 2}
```

Class names use the order `name` pattern and caps must be at least 1. An order
whose `class` is unset or not listed is limited only by `cap`. Admission for
`start`, `retry`, `rework`, an ended-run `say` and an amend restart counts this
host's runs of the same class that are running, reserved, or launched but not
yet confirmed by their supervisor, plus class reservations held for
orchestrator source attempts. It counts every caller alike: direct CLI starts,
orchestrators, source rules and bound senders. A start that finds its class
full fails with `class NAME full (n/cap)`; a full class does not queue an
order by itself. The check runs under the same admission lock as `cap`, and a
run never counts against itself.

An order queued on a full receiver (see
[Full receivers and the FIFO queue](#full-receivers-and-the-fifo-queue)) holds
no class slot while it waits. Dequeue admits its class under the admission
lock like any start. A queue head whose class is full is not refused: it stays
at the head, later entries wait behind it, and it starts once the class frees.

Changing an order's `class` with `amend` restarts a live run, so the new class
is admitted like a launch and the old slot is released only once the worker
has stopped.

An `agent: off` source rule admits and reserves its order's class slot on the
target, under the target's admission lock, before it writes its `attempting`
receipt and starts the run. This holds on any target host, local or reached
over SSH, so the slot is held on the host that runs the work. The run's start
consumes the reservation. Any definitive start rejection releases it at once,
whether the orchestrator was paused or stopped, its budget was exhausted, or
setup failed before the run directory existed. A start that finds the host
full is queued instead, and the queued order releases the reservation. Each probe's
reservation carries a fresh token that its start presents, so a start
releases only its own probe's reservation: a delayed or stale start never
frees the slot a later retry of the same run reserved. Otherwise,
for example after a lost start, it holds the slot until the source attempt
grace (15 minutes) passes, when the owner's receipt stops being an attempt. A
target too old to reserve a class is refused rather than bypassed: the owner
records the receipt as rejected and retries it later.

## Repository admission

Each target can restrict the repositories it admits with a top-level policy in
its own default configuration:

```toml
cap = 2 # target capacity

[hosts.buildbox]
local = true
cap = 2 # optional sender ceiling for this route

[admission]
repos = ["https://example.com/team/project.git", "ssh://git@example.com/team/another"]
```

`admission.repos` matches the target clone's `origin` remote, never the order's
repository label or directory name. SSH (including `git@example.com:team/project`)
and HTTPS spellings match; user names, transport, trailing slashes and a trailing
`.git` are ignored. Server names are case insensitive, repository paths are case
sensitive. Explicit ports remain significant. Local remote paths are also supported;
Windows local paths (including UNC paths) compare case insensitively across the
entire path; separators are normalized. URL paths remain case sensitive. Configure
these paths as they appear on the target. A missing `repos` key admits all repositories; `repos = []`
admits none. `add_from` lists origin prefixes from which teammates granted the
`repos` capability may add and remove repositories with `hev admission add`
and `hev admission remove`; see
[teammate-added repositories](nodes.md#teammate-added-repositories).

The target reads this policy from its own configuration before fetching or creating
any branch or worktree, including validator admission. It also checks the current
policy before rework or an ended-run `say` relaunches. Policy is always read from
the target process's `HEV_DISPATCH_CONFIG` or `~/.config/hev/dispatch.toml`,
independent of the sender's host alias, `--config`, or `config_path`. Those sender
settings select launch configuration only; they cannot select admission policy.
A restricted target also refuses a clone whose origin cannot be read. Refusal exits with code 1 and one `Error:` line naming the repo and host.
The failed admission record retains the reason in `show`, `ls --all`, and their
JSON output; `clean` removes the record.
Refused rework and ended-run `say` leave the completed run's result unchanged,
including its PR and validation eligibility. Legacy `hosts.NAME.admission` keys
are errors; move them to top-level `[admission]` in the target default config.
Policy enforcement starts in 0.6. A target running a pre-0.6 `hev` does not
enforce this policy; upgrade every receiving binary before relying on it.

Project allow/deny filters are unsupported: the old `[projects]` table and `HEVD_PROJECT_ALLOW`/`HEVD_PROJECT_DENY` environment variables are accepted and ignored with a warning when configuration loads, because they never filtered capture. Remove these obsolete settings; use `[capture] scan_roots` to select transcript roots. Warnings are not emitted per scan.

## Validation configuration

In the dispatch TOML config, select the browser validator independently of the
implementation worker:

```toml
[validation]
host = "browser-worker" # a configured [hosts.browser-worker]
harness = "codex"       # codex or claude
model = "your-validator-model"
allow_same_host = false
```

Orders with `validation: browser` request a validation run after implementation.
The validator requires a separate host by default and a different harness/model
pair. `hev validate NAME` (also `hev dispatch validate NAME`) requests a pass explicitly;
`--allow-same-model` permits the implementation's harness/model pair. Validation
writes verdict and evidence records; inspect them with `hev dispatch show NAME`.
## Amend an order

Use `hev dispatch amend NAME revised.md` to replace the full order while keeping
its name, branch and PR. `hev dispatch amend NAME -` reads from stdin; omitting
the file also reads stdin. Frontmatter and `## Acceptance` are required just as
for a new dispatch. The name and repository cannot change.

Every changed amendment saves the previous order and a line diff in
`history.json`. `hev dispatch show NAME` lists numbered revisions and attempts;
`show NAME --json` exposes them under `history.revisions` and `history.attempts`,
including each attempt's launch settings and result/trace session. Old logs and
reports remain archived in the run directory. Traces retain the original order
name across attempts.

For a live run, body edits update `order.md` and append the diff to `inbox.md`.
The worker reads that inbox between steps. `timeout`, `reply`, and `validation`
are updated in place; the supervisor reloads them while running. Timeout is
measured from the start of the current attempt, so reducing it below the elapsed
time ends the attempt promptly. Changes to `host`, `harness`, `model`, or `base`
stop the current attempt and start the next one on the existing branch. Changing
`base` updates the worker's instructions; it does not reset or rebase its branch.
An ended run always starts another attempt using the revised full order.

A host move checks the new target's repository admission and capacity before
stopping the worker, then checks them again during import. The original owner
must have a route to the new host in its dispatch configuration. The destination
needs the repository clone in its configured workspace and an unused order name
and branch. Committed branch history, working files (including uncommitted work),
inbox and run history travel together. Launch wrappers and environment names are
resolved afresh on the destination. The original owner retains its record and
worktree and forwards run commands and native peer messages, so the original
`HOST/NAME` peer address continues to work. Keep that owner reachable. Imported
attempts are hidden from dispatch inventory to avoid duplicate order names.

An admission refusal during the initial check leaves the original record and
worker intact. A definitive import refusal restores the source as a stopped run
with its original record, ready for rework or another amendment. Failed imports
clean up target files and branches before launch. If a transfer connection fails
or launch is uncertain, the source keeps forwarding and reports that the target
needs inspection. An identical amendment is a no-op. Inspect `show` on the destination before retrying an
uncertain transfer; never start a second worker on a guess.

Choose the command by what changed:

- `amend` revises the actual order, acceptance criteria or launch settings.
- `say` sends a note outside the order to a live worker. On an ended run it starts
  rework with that note, preserving its existing settings. `say --resolve KEY`
  answers the run's open decision (see [Run outcomes and decisions](#run-outcomes-and-decisions)).
- `rework` starts another attempt of an ended run and appends a `## Rework` note;
  it retains the saved host, model and other launch settings.

### Repeating orders

Set `every: 15m`, `until: plain text condition to check`, and optionally
`max_lifetime: 24h` in frontmatter. Durations use Go duration syntax. The first
round starts immediately. Subsequent rounds follow the preceding start's cadence;
missed ticks are skipped, so rounds never overlap. The detached supervisor works
on Linux, macOS and Windows and holds one host slot for the entire run, including
waiting periods, keeping admission predictable.

Each round starts a fresh harness session in the same worktree. It reads
`inbox.md` and `round-notes.md` first, and updates the notes for the next round.
Peers messages, chats and `dispatch say` accumulate in the inbox while waiting;
senders see `queued for next round at HH:MM` (UTC). `schedule.json`, `ls` and
`show` expose the round count and last/next start. Logs retain the order name
through the run directory; previous rounds are saved as `round-N.log`.

The worker checks the `until` text and writes `until-met.json` with
`{"met":true,"reason":"evidence"}` when satisfied. This ends scheduling after
the round. Maximum lifetime also bounds active work. `dispatch stop` stops an
active round or ends a waiting run. Amendments to task text, cadence and stop
conditions apply on the next round; harness, model, base or host changes retain
normal amend restart behavior.

Repeating runs send one final completion notification with the end reason
(`until`, `max_lifetime`, `stopped`, or `repeat removed`); individual
rounds do not send completion notifications. Supervisor restarts and amendments
retain round numbering, logs, and the original lifetime deadline. Rework of an
ended repeating run also retains that deadline and history; use a new order name
to start a new lifetime. Existing round logs are never replaced.

For optional repeating review and follow-through on this session's open runs,
see [expedite](expedite.md).

## Ask traces

Every run belongs to the ask that started its work. `hev dispatch ORDER.md`
mints a 128-bit W3C `trace_id` and a root ask span, and records both in the
run's `meta.json` with what it can learn about the caller: the tmux session
(a FOH session name, or `HEV_CALLER_SESSION`), the harness and its session ID
(`CLAUDE_CODE_SESSION_ID`, `CLAUDE_SESSION_ID` or `CODEX_THREAD_ID`), host and
time.

- A plain `hev dispatch ORDER.md` (or `dispatch run`) starts a new trace.
- `--trace ID` continues an existing trace with a follow-up ask. `ID` is a
  32-hex trace ID, or a W3C traceparent to also name the span it continues
  from. A `trace:` frontmatter key does the same; the sender removes it before
  the order leaves, so targets never see it.
- `rework`, `retry`, `say`, `amend` and `validate` on a traced run record a
  follow-up ask in the same trace, with the note (or the amend diff) as its
  text. A `say` to a live run is an ask without a new attempt.
- Inside a traced worker, `hev dispatch` inherits that worker's
  `TRACEPARENT`. An orchestrator launch with no trace context, and a source
  event run, are recorded as untraced with the reason in `untraced`. A trace
  is never invented from an order name.

Each launch of a traced run mints an attempt span (`span_id`, parented by the
ask in `parent_span_id`), copied into `history.json` and the attempt's
`result.json`. The supervisor starts the worker with `TRACEPARENT`
(`00-<trace_id>-<span_id>-01`), `HEV_TRACE_ID`, `HEV_SPAN_ID` and `HEV_RUN`,
replacing any trace variables it inherited. A browser validation run joins the
source run's trace under the validate ask, or under the attempt it validates.
All trace fields are optional: an older hev ignores them. A record with no
trace fields and no `untraced` reason was written before tracing (or by a hev
older than 0.7); the dashboard labels it **before tracing**, as history, not
as a fault. **untraced** is kept for runs recorded as untraced on purpose, and
always shows the reason.

The indexer stamps worker session rows with `trace_id` and `parent_span_id`
from the run record. The dashboard's **Asks** tab lists traces: root ask,
time, user, hosts, project, orders, runs, state, cost and duration.

- **User** is who the run was dispatched for: `requested_by`, else `sender`,
  shown without the `local:` prefix (`local:hev` reads as `hev`).
- **Hosts** reads asking host → execution hosts, such as `laptop → mini,
  oven`. The asking host comes from the ask's caller, and the execution hosts
  include validation runs. A run with no recorded ask shows only where it
  ran.
- **Project** is the order's repository, by name.

Filter the list by user, by host (asking or execution) and by project. The
filters combine. Each choice is counted with the other filters applied, so
none leads to an empty list. They are kept in the page URL as `ask_user`,
`ask_host` and `ask_project`, so a filtered list can be linked, and they work
the same on rows read from other hosts. Runs from before tracing are hidden
by default; the list counts them, and the **before tracing** control
(`ask_pretrace=1`) shows them. They are hidden by default because they have
no ask, which is what this view is about, and on a host with a long dispatch
history they fill the list. The list has no order filter, since nearly every
ask has its own order; the trace page keeps one. The API is `GET
/api/asks?window=&user=&host=&project=&pretrace=1`, where a repeated
parameter means either value.

The Asks list opens each trace as a by-ask
waterfall of asks, orders, attempts, validation runs and sessions, expandable
to turns. Its header shows the same user, project and asking → execution
hosts. Selecting an ask, order, attempt or validation row opens a detail
panel: the full ask text and caller, the order's intent and Acceptance, the
attempt's state, timings, branch, PR and worker report, and the validation
verdict. A worker session opened from a trace shows a breadcrumb whose every
part links back: the trace, the launching ask and the run (each scrolled to
and highlighted, as `#ask/<trace>/span/<span>`), and the order (as the order
filter, `#ask/<trace>/order/<name>`).

Traces work across hosts. Each host's indexer (`hev index` and the daemon's
cycle) publishes the trace facts of its own run records to the archive's
`<namespace>-spans` collection: one row per ask and one per attempt, keyed by
span ID, rewritten only when a row changes. The dashboard derives the model
from this host's run records plus the rows other hosts published, so a
dashboard on the mini shows runs that live on Oven. A run the dashboard's
host holds itself is read from disk. With no archive, or no rows yet, it
reads local records only.

A row carries: span, trace and parent span IDs; the run name and a run key
(name plus first queued time); the publishing host; times; for an ask, its
action, kind, text and caller (session, harness, harness session, host,
sender, run); for an attempt, its number, revision, the validated run's
name, the order's title line, repo, branch, head, PR, host, harness, model,
state facts (exit code, outcome, validation state), the worker session ID,
the untraced reason, the requester (`requested_by`, optional) and the run's
validation names and states. Ask text and
titles pass through the redaction scrubber first. Order bodies, Acceptance,
reports, notes beyond the ask text, errors, verdict items and environment
are never published, so the detail panel shows those only for runs on the
dashboard's own host.

Spans are derived at read time; nothing beyond these rows is stored. To try
it on a fixture:

```sh
HOME=$(mktemp -d) hev serve --demo-traces --port 8787   # then open /#asks
```

## Full receivers and the FIFO queue

A full receiver accepts the order into `~/dispatch/.queue/` and prints its queue
position. New orders wait behind existing queued orders. With no selected host,
dispatch first looks for a reachable host with free capacity.

Use `hev dispatch ls` to see queued orders, their positions and age. Use
`hev dispatch show NAME --host HOST` to inspect the wait, and `hev dispatch stop
NAME --host HOST` to cancel an order that has not started.

Owners set top-level `cap` in the receiver's dispatch configuration. Finishing
supervisors start the next waiting order. Keep the existing hevd scan running
as a crash-recovery backstop, or add `hev dispatch sweep` to the host's existing
scheduled task (the same command works on Unix and Windows). Sweep uses the
receiver's default dispatch configuration, including `HEV_DISPATCH_CONFIG` when
set. It opens no listener and needs no separate queue process. Failed setup
stays visible as a failed run; inspect it before explicit rework.

The initial queue is FIFO only. Sender shares, expiry, opt-out and start notices
are deferred; see [RFC 0012](rfcs/0012-dispatch-queue.md).

### Backstop receiver environment

The daemon invokes the installed `hev dispatch sweep` from its PATH rather than
launching workers with the daemon executable. Set `HEV_DISPATCH_RECEIVER` in the
daemon's service environment to an absolute receiver executable or wrapper when
PATH does not select the intended installation. On Windows the existing scheduled
task should invoke that same receiver entry point.

A service cannot inherit a previous SSH session's environment. Configure the
receiver wrapper (and the host's `harness_wrapper`) to load required credentials
at use time. Keep the receiver config path and credential provider consistent
between interactive dispatch and service recovery. Queue files store no secret
values. Updating the receiver does not require updating the daemon binary.
Admission refusals after enqueue (including paused or budget-exhausted
orchestrators) become visible failed run records; later FIFO entries proceed.
A full class is not a refusal: the head waits for its class slot (see
[Dispatch classes](#dispatch-classes)).
An owner can rework a refused order after resolving its admission policy.
