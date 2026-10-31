# Dispatch agent work

`hev dispatch` sends an order to a configured machine and leaves a durable run
record under `~/dispatch/<name>/`. Dispatch works without a daemon. Trace
capture still requires the normal capture/indexing setup.

Create `~/.config/hev/dispatch.toml` on the machine sending work. Configure `notify` separately on each target;
the supervisor reads its target configuration again at completion:

```toml
workspace = "~/workspace"
notify = ["notify-command", "{reply}", "dispatch {name}: {state} {summary}"]

[validation]
host = "mac"
harness = "claude"
model = "your-validator-model"

[hosts.buildbox]
ssh = "buildbox"
cap = 8
namespace = "my-traces"

[hosts.mac]
ssh = "mac"
cap = 2
path_prefix = ["~/.local/bin", "/opt/homebrew/bin"]

[hosts.winbox]
ssh = "user@winbox"
cap = 3
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
timeout: 120
---
Improve the help text.

## Acceptance
- Help explains the new command and tests pass.
```

`name` must match `^[a-z0-9][a-z0-9-]*$`; `repo` is a directory beneath `workspace`, an
absolute path, or a `~/` path. `base`, `harness`, `validation`, and `timeout`
default to `main`, `codex`, `none`, and 120 minutes. `claude` is the other
harness; `browser` is the other validation value. `model`, `reply`, and
`issue`, and `preview` are optional; `linear` is accepted as an alias for `issue`. Unknown,
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
even after cleanup. Dispatch fetches `origin` and creates branch `d/<name>`
at `origin/<base>` in `~/dispatch/<name>/wt`. Setup failures record a failed result and release the reservation. SSH mutations
allow five minutes for the two bounded Git operations. A lost connection reports
an unknown outcome: inspect the named record before retrying. Requests travel on
stdin so large order files do not exceed command-line limits. `clean` uses Git's normal worktree removal checks and keeps
the order, status, report, logs, metadata, and result; it does not delete the
branch or remote PR.

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

During indexing, sessions and searchable chunks whose working directory is
under `~/dispatch/<name>/` receive `order=<name>`. The order file supplies an
optional `issue` value. This requires indexing with a version that supports
these attributes; older indexed traces need reindexing.

```sh
hev traces ls --order improve-help
hev query 'why did it fail' --order improve-help
```

A host's optional `namespace` is passed as `LAYER_NAMESPACE` in the worker's
environment. Configure the capture daemon's destination consistently with
that namespace. Dispatch does not start, restart, or reconfigure capture.

Unix launchers forward a defined environment allowlist (including harness API
keys and authentication paths) through tmux rather than inheriting its stale
server environment. Target `[hosts.<name>.env]` keys extend or override it.
Values are never written to the record or dispatch logs. `logs -f` transfers
only new bytes from a tracked offset. `traces ls --order` searches all ages
unless a time window is explicitly selected.

Windows launches with job breakaway; if SSH disallows breakaway, it uses a
one-shot scheduled task. The harness starts suspended, joins a kill-on-close
job, and only then resumes, retaining
control of descendants even when the root harness exits. Its admission lock is
a temporary directory; if a dispatcher crashes during admission, inspect the
record and determine whether an admission operation is still active before
recovering a stale lock.

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
