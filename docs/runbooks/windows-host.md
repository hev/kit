# Run hev as standing automation on a Windows host

This runbook sets up one Windows machine, here `winhost.example` (dispatch
alias `winhost`), so that it receives dispatched work, runs an orchestrator
unattended, and reports to an owner on another machine. The owner can ssh to
the host. The host may not be able to reach the owner, and it runs no peers
session of its own.

Capture setup (`hev daemon install`) is covered in the
[Windows guide](../windows.md). The orchestrator spec format is in
[orchestrators](../orchestrators.md).

## What is shipped and what requires kit#159

| Piece | Status on kit main |
|---|---|
| `hev.exe`, dispatch to a Windows target, `harness_wrapper` | Shipped |
| Orchestrator `apply`, rounds, checkpoints, `notify_route: pull` | Shipped |
| `hev orchestrate supervise NAME` (hidden; one supervisor per orchestrator) | Shipped |
| A supervisor started by `apply` over ssh outliving the ssh session | **Requires kit#159** |
| Crash recovery for the root lock (`.lockdir` is left behind by a crash) | **Requires kit#159** |
| Retrying renames under a concurrent reader, Windows round identity | **Requires kit#159** |

On kit main, a supervisor that dies while holding the root lock leaves
`~\dispatch\.lockdir`, and every later lock times out with
`admission lock busy; inspect ...`. A scheduled task that restarts
`supervise` does not clear it. Until kit#159 merges, treat the scheduled task
below as a reboot convenience, not crash recovery, and check `hev orchestrate
ls` after any unexpected exit.

## 1. Install

1. Download `kit_<version>_windows_amd64.zip` from GitHub Releases, verify it
   against `checksums.txt` and extract `hev.exe` into a permanent directory on
   the task user's PATH, for example `C:\tools`. Scheduled tasks record
   absolute paths, so keep the executable in place and replace it in place on
   upgrade.
2. On the host, create `%USERPROFILE%\.config\hev\dispatch.toml`. Top-level
   keys set receiving capacity, workspace and run notices; the launch wrapper
   is a host key, so it goes in the host's own entry, under the same alias
   the owner uses:

   ```toml
   cap = 2
   workspace = "C:/src"
   notify = ["pwsh.exe", "-NoProfile", "-File", "C:/tools/notify-owner.ps1", "{reply}", "dispatch {name}: {state} {summary}"]

   [hosts.winhost]
   local = true
   os = "windows"
   harness_wrapper = ["pwsh.exe", "-NoProfile", "-File", "C:/tools/with-creds.ps1"]
   ```

   A top-level `harness_wrapper` is refused (`unknown config key`).

3. On the owner machine, add the host. The target's own wrapper always
   applies; a sender's wrapper is used only when the target has none.

   ```toml
   [hosts.winhost]
   ssh = "user@winhost.example"
   os = "windows"
   workspace = "C:/src"
   harness_wrapper = ["pwsh.exe", "-NoProfile", "-File", "C:/tools/with-creds.ps1"]
   ```

4. Restrict `%USERPROFILE%\dispatch` to the task user before the first apply.
   Go file modes do not set Windows ACLs, and rendered orders and round
   packets live there.

   ```powershell
   icacls "$env:USERPROFILE\dispatch" /inheritance:r /grant:r "${env:USERNAME}:(OI)(CI)F"
   ```

## 2. Credentials at launch time

Windows does not forward environment variables across scheduled tasks, and
dispatch forwards none to a Windows target. Every process that needs a secret
fetches it when it starts, from a vault, through a wrapper.

There are two launch points:

| Process | Gets credentials from | When |
|---|---|---|
| Dispatched workers and validators | The host's `harness_wrapper` | Once per run |
| Orchestrator rounds | The environment of `hev orchestrate supervise NAME`, which the task starts through the same kind of wrapper | Once per supervisor lifetime |

A round harness inherits the supervisor's environment, so a token that the
round needs (for example a GitHub token for `gh pr view`) must be present when
the supervisor starts. With `idle: skip`, an unchanged round makes no model
call and reads no secret.

The wrapper contract and a complete PowerShell 7 wrapper are in
[dispatch](../dispatch.md): consume `--`, pass every later argument verbatim,
wait, and return the child's exit code. Start from that `with-creds.ps1` and
set the child's environment before `Process.Start`, with `vault-cli` standing
in for your vault's CLI:

```powershell
# Values go into the child's environment only. Never print or log them.
$start.Environment['GH_TOKEN'] = (& vault-cli read 'vault://example/github/token').Trim()
```

Rules that keep secrets out of transcripts and quotas intact:

- Read secrets inside the wrapper, into variables. Never make a secret-reading
  command the wrapper's direct child: the wrapper passes its child's stdout
  through to whoever launched it, which may be an agent transcript.
- Do not pipe or capture the wrapper's output in a calling script to inspect a
  secret. Print only derived facts (lengths, an identity check).
- Read each secret once per launch. A loop that polls every minute and reads a
  secret each time can exhaust a vault's account-wide rate limit for every
  machine sharing it. Polls inside a running supervisor read nothing.
- Put no secret in wrapper arguments, task definitions or `dispatch.toml`.

## 3. Supervise under a scheduled task

Apply each spec on the host first. Specs for this host should set
`notify_route: pull` (section 4); see
[examples/windows-follow-through.md](examples/windows-follow-through.md).

```powershell
hev orchestrate apply -f C:\orch\winhost-follow.md --dry-run
pwsh -NoProfile -File C:\tools\with-creds.ps1 -- hev.exe orchestrate apply -f C:\orch\winhost-follow.md
```

`apply` starts a supervisor itself, and so can `say`, `wake`, `resume` and
`handoff` when none is running. That supervisor inherits the caller's
environment, and the scheduled task cannot replace it while it lives. Run
these commands through the wrapper, as above, or the rounds of that
supervisor run without credentials.

Then register one task per orchestrator that runs
`with-creds.ps1 -- hev.exe orchestrate supervise NAME`. `supervise` admits
one supervisor per orchestrator: a second copy exits non-zero with
`orchestrator already running` and changes nothing, and a copy started for a
stopped or complete orchestrator exits at once. That makes a repeating trigger
safe.

```powershell
$name = 'winhost-follow'
$action = New-ScheduledTaskAction -Execute 'pwsh.exe' `
  -Argument "-NoProfile -File C:\tools\with-creds.ps1 -- C:\tools\hev.exe orchestrate supervise $name"
$triggers = @(
  New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
  New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes 5)
)
$settings = New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew `
  -ExecutionTimeLimit ([TimeSpan]::Zero) -StartWhenAvailable
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive
Register-ScheduledTask -TaskName "hev-orch-$name" -TaskPath '\hev\' -Action $action `
  -Trigger $triggers -Settings $settings -Principal $principal
```

| Setting | Value | Why |
|---|---|---|
| Triggers | At logon, and repeating every few minutes | Restart after reboot or exit |
| Multiple instances | `IgnoreNew` | A running supervisor makes the repetition a no-op |
| Execution time limit | Zero (no limit) | The default of 72 hours kills a healthy supervisor |
| Logon type | `Interactive` | Coding harnesses need a non-zero session (below) |

To change a spec: disable the task, `hev orchestrate stop NAME`, reapply
through the wrapper, then enable the task. Apply refuses while a supervisor is live.

**Session 0.** Services and tasks that run whether or not the user is logged
on (S4U, "do not store password") run in Session 0, with no interactive
token. That is fine for capture: `hev daemon install` uses S4U. Coding
harnesses and their sandboxes need a process in the user's interactive
session, with a non-zero session ID. Register supervisor tasks as
`Interactive` for a user who stays logged on (for example through automatic
logon on a dedicated machine). Processes that the Windows OpenSSH server
starts normally run in Session 0, so a worker dispatched over ssh may not get
an interactive session. Prove one coding run end to end on the host before
relying on it.

**Codex outside dispatch.** Dispatch and rounds launch Codex with
`--dangerously-bypass-approvals-and-sandbox`, so the task user's file ACLs are
the boundary. If you start Codex yourself from a task, use
`--sandbox workspace-write -c 'approval_policy="never"'`. An approval policy
that prompts stalls an unattended run until its time limit.

## 4. Owner notices for a host without peers

**Orchestrator notices.** With `notify_route: pull`, the host writes each
owner notice as a `queued` receipt and runs no transport. The owner pulls on
a timer:

```sh
hev orchestrate notices pull --host winhost             # list pending, JSON
hev orchestrate notices pull --host winhost --deliver   # deliver locally, then ack
```

`--deliver` puts each notice into the owner's local peers inbox under its
stable ID and then acknowledges it on the host. A lost ack redelivers the
same IDs, which the inbox deduplicates. Notices wait on the host while the
owner machine sleeps. Run it every minute from cron, a systemd user timer or
launchd. See [orchestrators: pull route](../orchestrators.md#pull-route).

**Run notices.** Dispatch runs `notify` from the host's own `dispatch.toml`
when a run finishes, with `{reply}`, `{name}`, `{state}` and `{summary}`
substituted. On a host without peers, point it at a command that can deliver
(for example a script that posts to a webhook the owner reads). A failed
notify is recorded as `notify_error` and never changes the run state. Without
a `notify` command, the owner polls `hev dispatch ls --host winhost`.

**Owner to orchestrator.** `hev dispatch say o-NAME "MSG" --host winhost`
reaches the orchestrator's inbox over the same ssh RPC and wakes a round.

## 5. Task Scheduler traps

- **Task names are kept.** A finished task and its job directory stay until
  deleted. A retry of a one-off task needs a new name: append the next unused
  `-retry-N` suffix to both the task name and its directory, and never
  overwrite the earlier attempt's directory.
- **Read `LastTaskResult`, not only `State`.** A non-zero result overrides a
  stale `Running` state. Task Scheduler may report the result as a signed
  integer; convert it before comparing, for example
  `[uint32]($info.LastTaskResult -band 0xFFFFFFFFL)`. In PowerShell the
  literal `0xFFFFFFFF` is `[int] -1`, so masking with it changes nothing.

  | Result | Meaning |
  |---|---|
  | `0` | Completed |
  | `0x41301` | Running |
  | `0x41303` | Has not run yet |
  | `0x41306` | Terminated; for a task with a time limit, it timed out |
  | anything else | Failed |

- **The default time limit is 72 hours.** Long-lived tasks need an explicit
  zero limit; one-off worker tasks need an explicit limit that matches the
  order's timeout.
- **Tasks do not report back.** Poll status when you expect completion.
- **Timestamps are local time.** Task Scheduler and file times use the host's
  time zone, not UTC. Convert before comparing with dispatch records.

## 6. PowerShell 5 traps

`powershell.exe` on a stock install is Windows PowerShell 5.1. Prefer
`pwsh.exe` (PowerShell 7) for wrappers. When a script must run under 5.1:

- **An environment variable set to `''` is removed.** `GIT_CONFIG_VALUE_0=''`
  disappears, so git sees a count with no value. Pass
  `git -c credential.helper= ...` instead.
- **Quoting through ssh breaks.** `$`, `|` and nested quotes do not survive
  `ssh host '...'`. Copy a `.ps1` to the host, give each call its own unique
  remote file name, run it with `powershell -NoProfile -File`, and delete it
  afterwards.
- **Parameter names match by prefix.** A wrapper script with a `-FilePath`
  parameter also binds `-File`, so `wrapper.ps1 -File child.ps1` collides with
  the wrapper's own argument. Pass the child as `-FilePath powershell.exe
  -ArgumentList @('-NoProfile', '-File', 'C:\path\child.ps1')`, or use the
  `--` convention above.
- **Native stdout is passed through.** A script that runs a native command
  without capturing it prints that command's output to the caller. Capture
  into a variable whenever the output could hold a secret.
