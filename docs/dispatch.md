# Dispatch agent work

`hev dispatch` sends an order to a configured machine and leaves a durable run
record under `~/dispatch/<name>/`. Dispatch works without a daemon. Trace
capture still requires the normal capture/indexing setup.

Create `~/.config/hev/dispatch.toml` on the machine sending work:

```toml
workspace = "~/workspace"
notify = ["notify-command", "{reply}", "dispatch {name}: {state} {summary}"]

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
validation: none
reply: your-notification-destination
issue: TASK-123
timeout: 120
---
Improve the help text.

## Acceptance
- Help explains the new command and tests pass.
```

`name` must match `[a-z0-9-]+`; `repo` is a directory beneath `workspace`, an
absolute path, or a `~/` path. `base`, `harness`, `validation`, and `timeout`
default to `main`, `codex`, `none`, and 120 minutes. `claude` is the other
harness; `browser` is the other validation value. `model`, `reply`, and
`issue` are optional; `linear` is accepted as an alias for `issue`. Unknown,
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

`--config PATH` selects an alternate configuration. `ls`, `show`, and `hosts`
support `--json`. Listings inspect hosts in parallel; an unreachable host
occupies one line. Without `--all`, `ls` shows running and unexpectedly dead
runs. Names are searched across hosts; use `--host` if a name is ambiguous.

Admission is serialized per target. A full host fails with `host full (n/cap)`
and other hosts' free slots; there is no queue. Existing names are refused,
even after cleanup. Dispatch fetches `origin` and creates branch `d/<name>`
at `origin/<base>` in `~/dispatch/<name>/wt`. Setup failures keep their record
for inspection. `clean` uses Git's normal worktree removal checks and keeps
the order, status, report, logs, metadata, and result; it does not delete the
branch or remote PR.

The supervisor owns timeout and completion independently of the agent. On
Unix it runs in tmux session `d-<name>`, which can be attached directly. Live
tmux sessions determine occupancy. Windows checks supervisor PID and process
creation time to avoid counting reused or stale PIDs. A stop request kills
the harness process tree and records `failed` with exit code 143. A timeout
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
recorded as `notify_error` and cannot change the run state. Browser validation
prints a follow-up hint; `validate NAME` is a stub describing the future plan.

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

Windows supervises the harness in a job object with kill-on-close, retaining
control of descendants even when the root harness exits. Its admission lock is
a temporary directory; if a dispatcher crashes during admission, inspect the
record and remove `~/dispatch/.lockdir` once no admission operation is active.
