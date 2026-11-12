# RFC 0013: Node manifests (compose new nodes as code)

Status: Proposed (2026-11-12). Design only; no code in this change.

## Summary

A node is described by one manifest file, composed from reusable modules, and
kit makes a machine match it. The manifest declares packages per OS,
repositories, managed files, services and schedules, identity and secret
references, agent harnesses and the skills role, dispatch capacity and
admission, checks, and the steps only a person can do. Four verbs work on it:

- `hev nodes plan` shows what would change and changes nothing.
- `hev nodes doctor` says whether the node matches, including human steps.
- `hev nodes apply` makes the changes, with a rollback receipt.
- `hev nodes bootstrap` prints a first-boot script for a cloud machine.

The engine (schema, verbs, per-platform drivers) is public kit code. Every
manifest lives in its operator's private repository. Kit runs natively on
macOS, Windows and Linux, uses each platform's own package manager and service
manager, and reuses the platform code kit already has for its capture daemon,
SSH access and secrets.

## Motivation

Growing a cluster today means hand-writing a provisioning kit per machine: a
bootstrap script, a converge script, a doctor script, a package list, launchd
plists or systemd units, a scheduled-task table and a list of repositories to
clone. One operator's estate has about a dozen such scripts per host. Most of
what they do is generic:

- install a package list with the native package manager;
- clone a fixed set of repositories if missing;
- link dotfiles and agent settings into place and keep a rollback receipt;
- render per-user services and timers and load them without racing the
  service manager;
- check that every declared service is loaded, that no undeclared one is,
  that disk has headroom, and that the logins a host needs still work;
- print the steps a person must do at the console, in order, and check later
  that they were done.

Each script re-learns the same traps (untrusted package taps, a harness whose
postinstall did not run, services that start without a PATH, a package whose
installer opens a GUI on a headless box). None of it can describe a Windows
machine. A new node takes hours, and the result is checked by a script
written for one machine.

Kit already does much of the platform work for its own daemon: it renders and
loads a launchd agent (`cmd/hev/launchd.go`), a systemd user unit
(`cmd/hev/systemd.go`) and a Task Scheduler task (`cmd/hev/task.go`,
`cmd/hev/service_windows.go`), secures files per platform
(`internal/platformops/files.go`), manages `authorized_keys` and its Windows
ACLs (`internal/nodes/nodes.go`), pins host keys through a short-code exchange
(`internal/nodes/short.go`), and resolves secrets by reference
(`docs/rfcs/0009-secrets.md`). This RFC turns that into an engine that works
from a declaration.

## Decision

These were decided by Adam before this RFC and are recorded as given.

1. **Engine in kit, declarations private.** Kit (public) gets the manifest
   schema and the verbs. A node's manifest lives in the operator's own private
   repo. Nothing with host addresses, vault paths, account names or service
   config goes into hev/kit, including in examples. Use invented names
   (`build.example.test`, `op://example/...`).
2. **Must support macOS and Windows natively**, and Linux. This rules out Flox
   and Nix as the base (Windows only through WSL2) and Ansible (cannot run from
   Windows). The engine is kit's own Go code.
3. **Native package backends:** brew (formulae and casks) on macOS, winget on
   Windows, apt on Linux. The manifest names packages per OS. Flox may be noted
   under future work as an optional macOS/Linux backend; it is not in v1.
4. **Cloud nodes use the same manifest.** Kit does not create VMs in v1. It
   emits a first-boot script for a manifest (cloud-init on Linux, PowerShell
   user-data on Windows) that installs `hev`, enrols with a one-use expiring
   invite code and runs apply. Whatever creates the machine (a provider CLI,
   Terraform, a console) is handed that script. A provider wrapper is future
   work. Cloud Macs are treated like a physical box: someone provides the
   machine, then apply.
5. **Read-only first.** Phase 1 is the schema plus `plan` and `doctor`, proven
   by describing existing machines with no drift reported. Phase 2 is `apply`,
   accepted by standing up a new node from a manifest alone. Apply on an
   existing live host is a deliberate cutover, never a side effect.

## Goals

- One manifest per node, in TOML like kit's other configuration
  (`internal/dispatch/config.go` loads `dispatch.toml` with the same TOML
  library), composed from modules that several nodes share.
- The same schema for macOS, Windows and Linux nodes. A module can serve all
  three because each package names its per-OS spelling.
- `plan` and `doctor` that never change the machine and can describe a host
  that was provisioned by other means, so existing hosts can adopt manifests
  without being touched.
- `apply` that is idempotent, refuses risky changes without an explicit flag,
  and records enough to roll back the files and service definitions it
  changed.
- Human steps as first-class, ordered items: apply stops at the first one that
  is due and prints it; doctor verifies it.
- No secret value in a manifest, a plan, a log, a receipt or a first-boot
  script.
- A first-boot script that turns a fresh cloud VM into an enrolled node with
  no secret in its user-data.

## Non-goals

- Creating cloud machines, choosing instance types or managing provider
  accounts. Kit emits a script; something else creates the VM.
- Sandboxing. A node runs agents as its account with that account's
  credentials, as RFC 0011 already states (`docs/rfcs/0011-nodes.md`, "Trust
  model and non-goals"). A manifest does not change that boundary.
- Managing Kubernetes workloads or clusters, and container stacks in general.
  A node can run a service whose command starts a Compose project, but the
  engine does not model containers, images or volumes.
- Installing or partitioning an operating system, disk encryption, firmware
  settings, or growing filesystems.
- Joining a network (VPN or mesh login). The package can be declared; the
  login is a human step.
- Upgrading every package on each run. Apply installs what is missing and,
  only with a flag, moves a pinned version.
- Configuration management for arbitrary third-party applications beyond
  placing files, setting a small typed set of OS settings, and running
  declared commands as checks.
- Replacing `hev up`, `hev skills setup` or `hev nodes invite/join`. The
  engine calls them.

## Vocabulary

- **Node**: one machine and one OS account on it that kit manages. A machine
  with two managed accounts is two nodes.
- **Manifest**: the file that describes one node (`*.node.toml`). It names
  the node, its OS and account, and the modules it includes.
- **Module**: a reusable file with the same schema minus `[node]`, for
  example `base`, `harness`, `gui` or `ci-runner`.
- **Resource**: one keyed item in the resolved manifest, such as
  `packages.gh`, `services.skills-sync` or `human.screen-recording`.
- **Backend**: the native tool a resource kind is applied through (brew,
  winget, apt, npm, launchd, Task Scheduler, systemd).
- **Drift**: a resource whose observed state differs from its declared state,
  or a resource apply owns that is no longer declared.
- **Owned**: created or adopted by apply and recorded in the node's state.
  Apply changes and removes only owned things.
- **Receipt**: a record written by each apply of every file and service
  definition it replaced, with the prior contents, so the change can be
  rolled back.
- **Human step**: a declared action that only a person can do, with a check
  that shows it was done.
- **Privileged**: needs root, sudo or an elevated Windows token.
- **Cutover**: the first apply on a host that already runs services created
  by other means. It is a separate, explicit act.
- **First-boot script**: the cloud-init or PowerShell user-data that
  `hev nodes bootstrap` prints for a manifest.

## Proposed surface

### The manifest

A manifest is TOML with a schema version, a `[node]` table and keyed tables of
resources. Every resource table is keyed by a stable name, so composition
merges by key and plan output names every change.

```toml
schema = "hev.node/v1"

[node]
name = "build.example.test"   # stable label; not resolved as an address
os = "linux"                  # macos | windows | linux
arch = "amd64"                # amd64 | arm64; checked against the machine
account = "builder"           # the OS account services run as
label_prefix = "test.example" # service names kit owns: launchd label prefix,
                              # Task Scheduler folder \test.example\, unit prefix
modules = ["modules/base.toml", "modules/harness.toml"]
```

`~` in any path is the node account's home: `$HOME` on macOS and Linux and
`%USERPROFILE%` on Windows, matching how kit already places its state
(`docs/windows.md`, "State lives under `%USERPROFILE%\.hev`"). Paths use
forward slashes on every OS. Relative `source` paths resolve against the file
that declares them, not the working directory.

#### Packages

One entry per logical tool. The entry names the package for each backend it
can come from; the engine uses the backend for the node's OS. A module that
omits a spelling for the node's OS is a plan error unless the entry sets
`os` to exclude that OS.

```toml
[sources.example-tap]
brew_tap = "example/tap"
trusted = true                # declared trust; see "Known traps"

[sources.mesh-vpn]
os = ["linux"]
apt = { uri = "https://pkgs.example.test/apt", suite = "stable", components = ["main"],
        key_url = "https://pkgs.example.test/apt/key.gpg",
        key_sha256 = "0000000000000000000000000000000000000000000000000000000000000000" }

[packages.gh]
brew = "gh"
winget = "GitHub.cli"
apt = "gh"
check = ["gh", "--version"]

[packages.chrome]
os = ["macos", "windows"]
brew_cask = "google-chrome"
winget = "Google.Chrome"

[packages.claude-code]
npm = "@anthropic-ai/claude-code"   # cross-platform; needs packages.node
after = ["packages.node"]
check = ["claude", "--version"]     # proves the postinstall ran
```

Fields: `brew`, `brew_cask`, `winget`, `apt`, `npm` (one or more; each takes
a name or a list of names that install and check together, such as a vendor's
engine, CLI and plugins), optional `version` (honoured by winget, apt and npm; brew installs the current formula
and doctor reports the installed version), `check` (an argv run with a
deadline under the service environment; exit 0 means the tool works),
`install` (`auto`, default; `human` for packages that cannot install
unattended; `privileged` when the backend needs elevation, which apt always
does), `os`, `after`, and `transport` (marks the package that carries remote
sessions; see `--allow-network-change`).

#### Repositories

```toml
[repos.kit]
path = "~/workspace/kit"
url = "https://github.com/example-org/kit.git"
admit = true        # append to [admission] repos in this node's dispatch.toml
```

Apply clones a missing repository and never touches an existing one. Doctor
reports a missing clone, a different `origin`, and (as a warning) a dirty
tree. Fast-forwarding checkouts is not converge's job: moving code under
running services is a separate decision.

#### Files and links

```toml
[files.gitconfig]
path = "~/.gitconfig"
source = "files/gitconfig"   # relative to the declaring file
mode = "link"                # link | copy | seed
perm = "0644"
```

`seed` writes the file only when the path is absent and never replaces it
afterwards, for files a program edits after first install (kit's own
`~/.hev/config.toml` is one: it carries an archive target and redaction state
that must survive).

`link` makes a symbolic link on macOS and Linux. On Windows it copies, the same
choice RFC 0012 made for skills to avoid needing Developer Mode or an
administrator (`docs/rfcs/0012-skill-sources.md`, "Ownership and
publication"); doctor then compares content digests. `copy` writes the bytes
atomically. A file is never rendered from a secret (see open question 3).

Before replacing anything at `path`, apply records the prior state (absent,
a link and its target, or a regular file copied into the receipt directory)
in that run's receipt. `hev nodes apply --rollback RECEIPT` restores every
file in the receipt. A path that exists, is not owned and differs from the
declaration is not replaced unless the run has `--adopt` or `--cutover`.

#### Services and schedules

A service is a per-user daemon or schedule that runs one argv as the node
account. Its environment is exactly what the manifest says plus `HOME` and an
explicit `PATH`; it never depends on shell startup files.

```toml
[services.capture]
builtin = "hevd"             # kit's capture daemon, installed by kit's own code

[services.skills-sync]
command = ["~/.local/bin/hev", "skills", "sync"]
every = "1h"
log = "~/.local/state/hev/node/logs/skills-sync.log"

[services.report]
command = ["~/.local/bin/report-job", "--weekly"]
working_directory = "~/workspace/reports"
calendar = [{ weekday = 5, hour = 15, minute = 0 }]
secrets = { REPORT_TOKEN = "op://example/reports/token" }
limits = { cooldown = "20h", ceiling = { count = 1, window = "6d" } }

[services.heartbeat]
command = ["~/.local/bin/heartbeat"]
every = "5m"
state = "disabled"           # declared, written, never loaded; doctor checks it is not running
```

`builtin = "hevd"` installs kit's capture daemon through kit's own code and
accepts `secrets` and `log`; with secrets it becomes the daemon launcher that
RFC 0009 shows (`hev secrets run --ref ... -- hev daemon`). Other services set
exactly one of `keep_alive = true`, `every` or `calendar`. `calendar` entries
take `month`, `day`, `weekday` (0 is Sunday), `hour` and `minute`, the subset
that launchd's `StartCalendarInterval`, systemd's `OnCalendar` and Task
Scheduler's calendar triggers can all express, evaluated in the node's local
time. `secrets` maps environment names to references; the engine wraps the
argv in `hev secrets run --ref NAME=REF -- ...`, the integration RFC 0009
already describes for launchd, systemd and Task Scheduler
(`docs/rfcs/0009-secrets.md`, "Integration examples"). `limits` adds a durable
attempt journal: a run is skipped, and the skip logged, while inside the
cooldown or over the ceiling, and interrupted runs count. `run_at_load`
defaults to true for `keep_alive` and false for schedules.

Services run in the user's session domain, so they are subject to the same
login requirements kit's daemon already has: a launchd agent in `gui/UID`
(`cmd/hev/launchd.go`, `launchdDomain`), a systemd user unit
(`cmd/hev/systemd.go`) and an S4U Task Scheduler task (`cmd/hev/task.go`).
System-scope daemons (a vendor's monitoring agent, the SSH server, a VPN
daemon) are not services in v1; their packages install them, and a `check`
verifies they run.

#### Identity and secret references

```toml
[secrets]
provider = "1password"                  # resolver behind hev secrets run
token_file = "~/.config/op/token"       # placed by a human step, never by apply

[identity]
git_name = "Example Bot"
git_email = "bot@example.test"
github = { login = "example-bot", token = "op://example/bot/token" }
https_only = true
```

`github.token` is a reference. When `gh` reports a different login, apply
resolves the reference and passes the value to `gh auth login --with-token` on
stdin, then `gh auth setup-git`. It never puts the value in arguments, files or
output. `https_only` makes doctor report repositories whose `origin` uses SSH
to the forge and makes apply rewrite them to HTTPS. Every field of type
reference must parse as a reference (`op://...`); a literal value fails
validation, so a secret pasted into a manifest is rejected at plan time.

#### Agent harnesses and skills

```toml
[harnesses.claude]
package = "claude-code"   # a packages.* key
login = "human"           # creates human.harness-claude-login
check = ["claude", "--version"]

[harnesses.codex]
package = "codex"
login = "human"

[skills]
role = "host"             # host | sender, as in RFC 0012
machine = "builder"
sources = [{ url = "https://example.test/team/skills.git", ref = "refs/heads/main", subdir = "skills" }]
```

Apply writes `[skills]` into kit's config the way `hev skills setup` already
edits only that value (`docs/rfcs/0012-skill-sources.md`, "Setting up a
machine", step 1) and then runs `hev skills setup --role ROLE`, which installs
bundled skills and syncs sources. Doctor runs `hev skills check`. A harness's
login is always a human step: harness credentials live in places apply must
not write, such as the macOS login keychain.

#### Dispatch capacity and admission

```toml
[dispatch]
host = true                 # local = true in this node's dispatch.toml
cap = 3
classes = { heavy = 1 }
workspace = "~/workspace"

[dispatch.admission]
add_from = ["https://github.com/example-org/"]
```

These map one to one onto the existing `dispatch.toml` keys (`Config.Cap`,
`Config.Classes`, `Config.Workspace` and `Admission.AddFrom` in
`internal/dispatch/config.go` and `internal/dispatch/admission.go`). Repos with
`admit = true` are appended to `[admission] repos` with the same in-place,
decode-checked edit that `hev admission add` uses
(`internal/dispatch/admission_add.go`, `appendAdmissionRepo`). Sender-side
host entries and teammate access stay with `hev nodes invite`, `join` and
`accept`; a manifest describes the node, not who may use it.

#### OS settings

A small typed set, all privileged, chosen because headless nodes need them:

```toml
[settings.macos]
sleep = false
autorestart = true
remote_login = true

[settings.linux]
default_target = "multi-user.target"
mask = ["sleep.target", "suspend.target", "hibernate.target", "hybrid-sleep.target"]
linger = true              # user services run without a login session

[settings.windows]
openssh_server = true
sleep = false
```

Anything outside this set is a human step with a check, or a `check` alone.

#### Human steps

```toml
[human.secrets-token]
do = "Put the secret provider's service-account token on the node: hev nodes provide secrets-token (reads stdin, writes ~/.config/op/token with mode 0600)."
verify = { file = "~/.config/op/token", perm = "0600" }
before = ["secrets"]       # gates every resource that resolves a reference

[human.screen-recording]
os = ["macos"]
do = "At the console: System Settings > Privacy & Security > Screen & System Audio Recording, add /usr/libexec/sshd-keygen-wrapper."
verify = { command = ["screencapture", "-x", "/tmp/hev-node-check.png"], context = "ssh" }
```

`verify` is a file test (`file`, optional `perm`), a command with an expected
exit of 0, or a built-in probe (`autologin`, `filevault_off`,
`linger`, `openssh_server`). `context` says where the command runs: `service`
(the default, the same environment services get), `ssh` (a non-interactive
SSH session, for permissions granted to the SSH server's identity), or
`login`. `before` names resources or groups (`packages`, `secrets`,
`services`, ...) that cannot proceed until the step verifies; with no
`before`, the step gates nothing and is reported by doctor only.

`hev nodes provide NAME` is the one way to hand a value to a human step. It
reads stdin with echo off, writes the declared file atomically with private
permissions (`internal/platformops/files.go`, `WriteFileAtomic` and
`SecureFile`), and prints nothing derived from the value.

#### Checks

```toml
[checks.disk]
disk_free = { path = "/", min_gib = 60 }

[checks.docker]
command = ["docker", "info"]
timeout = "15s"
```

Checks are doctor-only. Each runs under a deadline enforced by kit, never by a
`timeout` binary. A command check passes on exit 0 and, when it sets
`expect`, only if its standard output also matches that regular expression,
so "prints `: On`" or "reports `API Key valid`" need no shell pipeline. The
same `expect` field is allowed in a human step's `verify`.

#### Composition and precedence

A manifest lists modules in `[node] modules`; a module may list its own
`modules`. Includes resolve relative to the including file, cycles are an
error, and depth is limited to 4. A module may declare
`[module] os = ["macos", "linux"]` and is then an error to include on another
OS.

Resolution:

1. Load every file. Each keyed resource (`packages.X`, `sources.X`,
   `repos.X`, `files.X`, `services.X`, `harnesses.X`, `human.X`, `checks.X`)
   and each singleton table (`[secrets]`, `[identity]`, `[skills]`,
   `[dispatch]`, `[settings.*]`) is a value with a provenance (the file that
   set it).
2. Modules merge by key. If two modules set the same key to different values,
   that is an error naming both files. Two modules setting an identical value
   is allowed.
3. The node file is applied last and may override any key: it replaces the
   whole entry, never a field inside it. It may delete an inherited key with
   `remove = ["services.heartbeat"]`.
4. Resource `os` filters drop entries for other OSes after merging.

`hev nodes plan --resolved` prints the resolved manifest with the provenance
of each key. The explicit-conflict rule keeps the outcome independent of
module order; open question 1 asks whether to keep it.

### The verbs

All four join the existing `hev nodes` group (`cmd/hev/nodes.go`): invite,
join, enroll (hidden), accept, pending, allow, deny, ls, remove.

```text
hev nodes plan      [-f FILE] [--host ALIAS] [--resolved] [--json]
hev nodes doctor    [-f FILE] [--host ALIAS] [--json]
hev nodes apply     -f FILE [--host ALIAS] [--privileged] [--cutover] [--adopt KEY]...
                    [--prune] [--upgrade] [--allow-network-change]
                    [--first-boot --enroll FILE]
hev nodes apply     --rollback RECEIPT
hev nodes bootstrap -f FILE --expires 1h [--hev-version VERSION]
hev nodes provide   NAME
```

**Why `hev nodes`.** The group already holds the commands that make a machine
part of someone's cluster: a host owner invites, a teammate joins, the owner
accepts. A manifest describes the host side of that relationship, and
bootstrap reuses join's short-code exchange. A new top-level `hev apply -f`
was considered and rejected: `apply`, `get` and `describe` are registered as
removed verbs that fail with a pointer to dispatch and orchestrate
(`cmd/hev/removed_verbs.go`), and `docs/design/resource-verbs.md` reserves
them for a future resource surface over dispatch and orchestrator state. Node
apply installs software and edits the OS, a different trust class that
should stay explicit. `-f FILE` follows `hev orchestrate apply -f FILE`
(`cmd/hev/orchestrate.go`). One cost: `hev nodes ls` lists access grants and
joins, not machines, so "nodes" means both the grant and the machine. Open
question 6 covers whether `ls` should show manifest state.

**Where each runs and what it writes.**

| Verb | Runs | Writes |
| --- | --- | --- |
| `plan` | On the node as its account. With `--host ALIAS`, the owner's machine sends the resolved manifest over the owner's own SSH to `hev nodes plan -f -` on the node. | Nothing. Observation uses read-only queries (`brew list`, `winget list`, `dpkg-query`, `launchctl print`, `systemctl --user show`, the Task Scheduler COM query kit already uses in `queryWindowsTask`, `cmd/hev/service_windows.go`). |
| `doctor` | Same as plan. Never remote unless `--host` is given; an environment variable cannot turn a local doctor into SSH. | Nothing. Exit 1 on any failure or drift. |
| `apply` | On the node as its account. `--privileged` runs the privileged phase through `sudo -n` (macOS, Linux) or requires an elevated token (Windows). `--host` is allowed for the user-scope phase. | Packages, files, service definitions, clones, `dispatch.toml` and config keys, and state under `~/.local/state/hev/node/`: `applied.json` (manifest digest and resolved copy), `owned.json`, `receipts/`, `limits/`, `logs/`, `apply.lock`. |
| `bootstrap` | On the owner's machine. Contacts no other machine. | The script on stdout; a dedicated SSH key pair under `~/.ssh` as `hev nodes join` creates (`internal/nodes/join.go`); an expected-node record with the code under the nodes directory (`~/.config/hev/nodes`, `internal/nodes/nodes.go`, `Default`). |
| `provide` | On the node. | The declared file of one human step. |

Exit codes for `plan` are 0 (no changes), 2 (changes pending) and 1 (error),
so scripts can tell drift from failure.

**Plan and doctor on existing hosts.** Phase 1 is accepted by describing
machines that were provisioned by private scripts. Plan and doctor therefore
compare meaning, not bytes: a launchd job matches when its label, argv,
environment, schedule and keep-alive match, whoever wrote the plist; a
systemd unit when its `ExecStart`, environment and timer match; a task when
its action and triggers match. Ownership markers matter only to apply. Doctor
also reports undeclared services under the node's `label_prefix`, so a job
someone loaded by hand shows up as drift.

**Apply order.** Validate; take `apply.lock` (`internal/platformops`,
`LockPath`); gate on human steps; privileged phase (settings, package
sources, apt and other privileged packages); packages; harness checks;
identity; files; repositories; skills; dispatch configuration; services;
then the doctor checks. Apply stops at the first unverified human step that
gates the next resource, prints that step's `do` text and its id, and exits 3.
Re-running apply after the step continues from there; nothing is skipped
silently.

**What apply refuses without an explicit flag.**

- `--privileged`: any change needing root or elevation. Without it, apply
  prints the privileged actions it would take and stops before them.
- `--cutover`: the first apply on a node whose state directory is empty while
  services under `label_prefix`, kit's own daemon, or declared paths already
  exist. This is how "apply on an existing live host is a deliberate
  cutover" is enforced. `--adopt KEY` adopts one existing resource instead of
  the whole node.
- Replacing a file, link or service definition that apply does not own.
  Needs `--adopt KEY` or `--cutover`; the prior state goes into the receipt
  either way.
- `--prune`: removing owned resources that are no longer declared. Without
  it they are reported as drift and left alone.
- `--upgrade`: changing an installed version to a declared `version`.
  Without it a version mismatch is reported, not fixed.
- `--allow-network-change`: installing, upgrading, reconfiguring or
  restarting the package or service that carries the current session (the SSH
  server, or a VPN daemon declared with `transport = true`) while apply runs
  over SSH.
- Writing a secret value anywhere, under any flag.
- Touching a dirty or diverged repository, under any flag.
- Removing packages. Rollback lists the package removals it would need and
  leaves them to the operator.

### Per-platform mapping

| Concern | macOS | Windows | Linux | Kit today | RFC |
| --- | --- | --- | --- | --- | --- |
| Package backend | brew formulae, casks, taps | winget (`--silent --disable-interactivity`, agreements accepted only when declared) | apt with signed sources, `DEBIAN_FRONTEND=noninteractive` and a lock timeout | None | New |
| Global npm packages (harnesses) | npm from the `node` package | npm from the `node` package | npm from the `node` package | None | New |
| Long-running service | LaunchAgent in `gui/UID`, `KeepAlive`, bootout then bootstrap with retry | Task Scheduler task, S4U logon, boot and logon triggers, restart on failure | systemd user unit, `Restart=on-failure`, linger | Rendered for kit's daemon only: `launchdJob.plist` and `ensure` in `cmd/hev/launchd.go`; `renderTaskXML` in `cmd/hev/task.go` with `scheduler` in `cmd/hev/service_windows.go`; `ensureSystemd` in `cmd/hev/systemd.go` | Generalise to any label, argv, env and log path |
| Schedule | `StartInterval`, `StartCalendarInterval` | Time and calendar triggers | `.timer` with `OnUnitActiveSec` or `OnCalendar` | Documented only as hand-written examples (`docs/rfcs/0012-skill-sources.md`, "Scheduling") | New, on the generalised renderers |
| Ownership marker | Plist bytes compared with the rendered plist (`launchdJob.ensure`) | Task folder `\hev\` (`windowsTaskName`, `cmd/hev/task.go`) | `# Managed by hev up` first line; refuses drop-ins and foreign units (`systemdOwnership`, `cmd/hev/systemd.go`) | As listed | Generalise: marker per node `label_prefix`, recorded in `owned.json` |
| Service environment | Explicit `PATH`, `HOME`, `HEV_CONFIG` (`jobEnv`, `launchdPath`, `cmd/hev/launchd.go`) | Config passed as an argument because tasks do not inherit the installing shell's environment (`renderTaskXML`) | `PATH` copied from the installing shell into the unit (`ensureSystemd`) | As listed | Generalise; Linux takes the declared PATH, not the installer's |
| Durable user services without login | Auto-login session (human step) | S4U task runs without a stored password (`docs/windows.md`) | `loginctl enable-linger` | None | New (`settings.linux.linger`; macOS auto-login is a human step) |
| Binary location | `~/.local/bin/hev` (`installSelf`, `cmd/hev/launchd.go`) | Absolute path recorded in the task; user picks the directory (`docs/windows.md`) | `~/.local/bin/hev`, ownership checked by build info (`systemdOwnership`) | As listed | Reuse; bootstrap installs to `%USERPROFILE%\.local\bin\hev.exe` on Windows, the directory RFC 0012 already uses for skill commands |
| SSH keys and ACL | `~/.ssh/authorized_keys`, 0600 in 0700 | Per-user `authorized_keys` or `administrators_authorized_keys`, protected DACL | As macOS | `AuthorizedPath`, `ACLArgs`, `secure`, `windowsACLScript` in `internal/nodes/nodes.go` | Reuse |
| Private files, atomic writes, locks | chmod 0600/0700, `flock` | Protected DACL, `LockFileEx` | As macOS | `internal/platformops/files.go`, `platform_unix.go`, `platform_windows.go` | Reuse |
| Secret resolution | `hev secrets run`, Unix socket 0700 | Same, per-SID named pipe | Same | `docs/rfcs/0009-secrets.md`, `cmd/hev/secrets.go`, `internal/secrets` | Reuse |
| Kit config and state | `~/.hev/config.toml`, `~/.config/hev/dispatch.toml`, `~/.config/hev/nodes` | Same under `%USERPROFILE%` | Same as macOS | `docs/windows.md`; `ConfigPath` in `internal/dispatch/config.go`; `Default` in `internal/nodes/nodes.go` | Reuse; add `~/.local/state/hev/node/` beside RFC 0012's `~/.local/state/hev-skills` |
| Skills | `hev skills setup` and `sync` | Same; copies instead of links | Same | `cmd/hev/skills.go`, `docs/rfcs/0012-skill-sources.md` | Reuse |
| Repository clone and admission | Bounded, hook-free clone; in-place admission edit | Same, clone in a job object | Same | `safeClone`, `appendAdmissionRepo` in `internal/dispatch/admission_add.go`; limits listed in `docs/nodes.md`, "Teammate-added repositories" | Generalise (clone from the manifest, not a teammate RPC) |
| Capture daemon | `hev daemon install` (launchd) | `hev daemon install` (Task Scheduler) | `hev up` (systemd) | `cmd/hev/daemon.go`, `cmd/hev/up.go` | Reuse as `builtin = "hevd"` |
| Platform facts | `runtime.GOOS`, `GOARCH`, cores | Same | Same | `pkg/nodeinventory/node.go`, `Observe` | Reuse for the `os` and `arch` check |
| First boot | Not supported: a person provides the Mac, then apply | PowerShell user-data | cloud-init | None | New |
| OS settings | `pmset`, `systemsetup` | power config, OpenSSH Server capability | `systemctl set-default`, `mask`, `loginctl` | None | New, typed set only |
| Host-key pinning for a new node | Short-code PAKE over SSH | Same; works with Windows OpenSSH | Same | `internal/nodes/short.go`, `cpace.go`, `join.go`; `docs/nodes.md`, "How short codes work" | Generalise: invitation seeded from a bootstrap code and bound to one owner key |

### Reuse, generalise or new

| Piece | Kit today | Verdict |
| --- | --- | --- |
| Secure files, atomic writes, file locks | `internal/platformops` | Reuse |
| `authorized_keys` paths, Windows ACLs, owned-line removal | `internal/nodes/nodes.go` | Reuse |
| Secrets by reference | `hev secrets run`, RFC 0009 | Reuse |
| Skills role and sources | `hev skills setup/check/sync`, RFC 0012 | Reuse |
| Dispatch cap, classes, admission keys | `internal/dispatch/config.go`, `admission.go` | Reuse |
| Capture daemon install | `cmd/hev/daemon.go`, `cmd/hev/up.go` | Reuse |
| OS and architecture facts | `pkg/nodeinventory` | Reuse |
| launchd agent renderer and loader | `cmd/hev/launchd.go` (daemon only, `KeepAlive` only) | Generalise |
| systemd user unit and ownership check | `cmd/hev/systemd.go` (one unit, `hevd.service`) | Generalise |
| Task Scheduler XML, quoting, UTF-16 encoding, query | `cmd/hev/task.go`, `cmd/hev/service_windows.go` (one task, `\hev\hevd`) | Generalise |
| Bounded repository clone and admission edit | `internal/dispatch/admission_add.go` | Generalise |
| Short-code enrollment and host-key pinning | `internal/nodes/short.go`, `join.go`, `cpace.go` | Generalise (seeded code, bound key, owner grant) |
| Package backends (brew, winget, apt, npm) | None | New |
| Schedules and attempt limits | None (`pkg/nativejob` is a fenced process bridge whose package comment in `pkg/nativejob/types.go` says it does not schedule work) | New |
| Managed files with receipts and rollback | None; RFC 0012's ownership manifest is the model | New |
| Manifest loader, composition, validation | None | New |
| Human steps, `provide`, checks, doctor | None | New |
| First-boot script emitter | None | New |
| OS settings | None | New |

### Human steps

Some steps cannot be automated on purpose: the platform requires a person, or
automating them would put a credential in the wrong place. The manifest
declares them, in order, with a check. Typical ones:

- macOS privacy grants (Screen Recording, Accessibility, Full Disk Access) at
  the console. They cannot be granted over SSH or by a script.
- macOS automatic login with FileVault off, for nodes whose user services must
  survive an unattended reboot.
- Joining the network: logging the machine into a VPN or mesh.
- Placing the secret provider's service-account token with
  `hev nodes provide`.
- Harness logins (`claude`, `codex`), which keep credentials where apply must
  not write.
- On Windows, the first elevated run that registers a startup trigger, which
  kit's own daemon install already needs (`docs/windows.md`, "Task Scheduler
  lifecycle").

Apply prints the first due step and exits 3. Doctor lists every step with
PASS or FAIL from its `verify`. Every step should have a `verify`. A step
that cannot be checked automatically must instead declare `evidence`, the
thing the person should produce, and doctor marks it `UNVERIFIED`, never
PASS.

### Known traps as checks

Provisioning notes from real hosts list traps that each cost hours and fail
silently. The generic ones become engine behaviour:

| Trap | Engine behaviour |
| --- | --- |
| A package manager refuses formulae from an untrusted third-party tap, and the bulk install reports partial failure that is easy to miss. | Tap trust is declared state (`sources.X.trusted`). Plan shows the trust action; doctor checks trust and checks every declared package individually instead of trusting one bulk exit status. |
| A harness installed through npm is a broken shim because its postinstall step did not run. | Package presence is not success. Each harness and any package with `check` runs that argv under the service environment; failure is drift. The npm backend runs installs with lifecycle scripts enabled and reports the package's own postinstall output on failure. |
| Non-interactive shells and services start with no PATH, so tools installed by a package manager are absent for SSH commands and scheduled jobs. | Every service gets an explicit environment from the manifest; nothing depends on shell startup files. Doctor runs `check` commands in `service` context by default, and checks marked `context = "ssh"` through a real non-interactive SSH session. |
| macOS has no `timeout` command (and Windows `timeout` is an unrelated wait command), so `timeout N cmd` fails as "not found", which looks like empty output inside a capture. | Kit runs every external command with a Go context deadline, as `scheduler` already does in `cmd/hev/service_windows.go`. Validation warns on any declared argv whose first element is `timeout` on macOS or Windows. |
| A cask whose installer is a GUI package blocks the bulk install forever on a headless machine. | Plan reads cask metadata and refuses casks that need a manual or GUI installer unless the entry is `install = "human"`. Every backend command runs with a deadline, non-interactive flags, and no attached terminal. |
| A tool that reads the login keychain works under the GUI session's services but reports "not logged in" over SSH, and exits 0. | Doctor checks harness logins in `service` context, not over SSH. Login checks look for a positive signal, not an exit status. |
| A check pipeline that stops reading early reports failure under `pipefail`. | Not applicable to Go checks. The engine captures command output and then matches it, never streams it into an early-exiting reader. |
| Replacing the network software over the network cuts the session doing the work. | `--allow-network-change` (see above). |
| A scheduled job fails or is skipped and nobody notices. | Every service has a log path, doctor reports each loaded job's last exit status, and a `keep_alive` service that is not running is a failure. |

### Secrets and safety

- A manifest holds references, never values. Reference fields are typed and
  validated. Literal strings in `env` are allowed, but validation rejects
  `env` keys whose names suggest secrets (`*TOKEN*`, `*SECRET*`, `*KEY*`,
  `*PASSWORD*`) unless they move to `secrets`.
- Plan, doctor and apply print references and never resolve them, except at
  the moment of use inside apply (the `gh` login) and inside
  `hev secrets run` at service start. Resolved values are never logged,
  passed as arguments, written to receipts or kept in state, following RFC
  0009's rules (`docs/rfcs/0009-secrets.md`, "Trust and storage").
- Receipts store copies of replaced files. A file can only be managed if it
  is declared with a `source`, and sources are files in the operator's
  repository; doctor fails a manifest whose source looks like a credential
  file (private key headers, token-like content).
- The one secret a node holds at rest, the secret provider's token, arrives
  through `hev nodes provide` and stays in the file the human step declares.

**What a first-boot script contains, and why it is safe as user-data.**
User-data is readable by any process on the instance through the metadata
service and by anyone who can read the instance's configuration in the cloud
account. The script therefore holds no credential. It contains:

1. The `hev` release URL for the node's OS and architecture and its SHA-256
   from the release's `checksums.txt` (published by
   `.github/workflows/release.yml`). The script refuses a mismatch.
2. The resolved manifest. It has no secret values by construction. It does
   carry the node's name and repository URLs; operators who consider those
   sensitive should not hand them to a provider, and that is their call.
3. The node account to create if missing, with no password (locked on Linux;
   on Windows, no stored password is needed for S4U tasks).
4. A one-use code with an expiry (default 1 hour, at most 24, as
   `hev nodes invite --expires` allows in `cmd/hev/nodes.go`) and the public
   half of the owner key `bootstrap` generated.
5. The commands: verify and install `hev`, seed the invitation, then
   `hev nodes apply --first-boot --privileged`.

The code's only power is to prove the node's identity to the owner. The
seeded invitation accepts exactly one key, the owner key whose public half is
in the script, so someone who reads the code cannot enroll their own key. What
they could do is pose as the node to the owner, which also needs a network
position between the owner and the node, or burn the code with wrong
attempts, which locks the invitation after 5 tries as it does today
(`docs/nodes.md`, "How short codes work"). Either way the owner runs
bootstrap again. The secret provider token, harness logins and network login
come later as human steps, over the pinned SSH connection.

**The enrollment flow.**

1. Owner: `hev nodes bootstrap -f build.example.test.node.toml > user-data`.
   This makes the dedicated key pair and the expected-node record, and prints
   the script. Kit never sees the provider.
2. The operator's own tooling creates the VM with that user-data.
3. First boot: the script installs `hev`, creates the account, seeds an
   invitation for the code bound to the owner key, opens the enrollment key
   line exactly as `hev nodes invite` does today (`openEnroll`,
   `internal/nodes/short.go`), and runs the first-boot apply until the first
   human step.
4. Owner: `hev nodes join USER@ADDRESS CODE`. Join recognises the expected
   record, uses the pre-made key, runs the existing exchange that pins the
   host key, and the node accepts the bound key immediately. The grant
   installed is an owner grant (an unrestricted key line), not a teammate's
   forced-command grant; see open question 5.
5. Owner: `ssh ALIAS hev nodes provide secrets-token < token`, then
   `hev nodes apply --host ALIAS -f FILE` until doctor is green.

A first-boot script for Linux looks like this (shortened):

```yaml
#cloud-config
write_files:
  - path: /var/lib/hev-node/node.toml
    permissions: "0644"
    encoding: b64
    content: c2NoZW1hID0gImhldi5ub2RlL3YxIgo=
  - path: /var/lib/hev-node/enroll.json
    permissions: "0600"
    content: |
      {"name":"build.example.test","account":"builder","code":"7K-Q4M2-XPWD",
       "expires":"2026-11-12T18:00:00Z",
       "owner_key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleOnly hev-owner"}
runcmd:
  - [sh, -c, "curl -fsSL -o /tmp/hev.tgz https://github.com/hev/kit/releases/download/vX.Y.Z/ARCHIVE && echo 'SHA256  /tmp/hev.tgz' | sha256sum -c - && tar -xzf /tmp/hev.tgz -C /usr/local/bin hev"]
  - [/usr/local/bin/hev, nodes, apply, -f, /var/lib/hev-node/node.toml, --first-boot, --privileged, --enroll, /var/lib/hev-node/enroll.json]
```

The Windows script is the same sequence in PowerShell: download the zip,
`Get-FileHash` against the embedded digest, expand it into
`%USERPROFILE%\.local\bin`, enable the OpenSSH Server capability, then the
same `hev nodes apply` line. Wrapping it in a provider's tags (for example
`<powershell>`) is the provider wrapper's job, which is future work. For
`os = "macos"` bootstrap refuses and says to install `hev` on the provided
machine and run `hev nodes apply`.

### Example: a macOS node

Modules first, then the node file. Every name, address and reference is
invented.

`modules/base.toml`:

```toml
schema = "hev.node/v1"

[sources.example-tap]
os = ["macos"]
brew_tap = "example/tap"
trusted = true

[packages.git]
brew = "git"
winget = "Git.Git"
apt = "git"
check = ["git", "--version"]

[packages.gh]
brew = "gh"
winget = "GitHub.cli"
apt = "gh"
check = ["gh", "--version"]

[packages.jq]
brew = "jq"
winget = "jqlang.jq"
apt = "jq"

[packages.tmux]
os = ["macos", "linux"]
brew = "tmux"
apt = "tmux"
check = ["tmux", "-V"]

[packages.op]
brew_cask = "1password-cli"
winget = "AgileBits.1Password.CLI"
apt = "1password-cli"
check = ["op", "--version"]

[sources.mesh-vpn]
os = ["linux"]
apt = { uri = "https://pkgs.example.test/apt", suite = "stable", components = ["main"],
        key_url = "https://pkgs.example.test/apt/key.gpg",
        key_sha256 = "0000000000000000000000000000000000000000000000000000000000000000" }

[packages.mesh-vpn]
brew = "tailscale"
winget = "Tailscale.Tailscale"
apt = "tailscale"
transport = true

[secrets]
provider = "1password"
token_file = "~/.config/op/token"

[human.secrets-token]
do = "Put the service-account token on the node: hev nodes provide secrets-token"
verify = { file = "~/.config/op/token", perm = "0600" }
before = ["secrets"]

[human.network-join]
do = "Log the node into the mesh VPN with a one-time auth key from the admin console."
verify = { command = ["tailscale", "status"] }

[services.capture]
builtin = "hevd"

[checks.disk]
disk_free = { path = "~", min_gib = 60 }
```

`modules/harness.toml`:

```toml
schema = "hev.node/v1"

[packages.node]
brew = "node"
winget = "OpenJS.NodeJS.LTS"
apt = "nodejs"
check = ["node", "--version"]

[packages.claude-code]
npm = "@anthropic-ai/claude-code"
after = ["packages.node"]
check = ["claude", "--version"]

[packages.codex]
npm = "@openai/codex"
after = ["packages.node"]
check = ["codex", "--version"]

[harnesses.claude]
package = "claude-code"
login = "human"

[harnesses.codex]
package = "codex"
login = "human"

[services.skills-sync]
command = ["~/.local/bin/hev", "skills", "sync"]
every = "1h"
log = "~/.local/state/hev/node/logs/skills-sync.log"
```

`modules/gui.toml`:

```toml
schema = "hev.node/v1"

[module]
os = ["macos"]

[packages.chrome]
brew_cask = "google-chrome"

[packages.claude-desktop]
brew_cask = "claude"

[human.autologin]
do = "System Settings > Users & Groups: log in automatically as the node account. FileVault must be off."
verify = { builtin = "autologin" }

[human.filevault-off]
do = "Turn FileVault off so an unattended reboot reaches the login session."
verify = { builtin = "filevault_off" }

[human.screen-recording]
do = "At the console: Privacy & Security > Screen & System Audio Recording and Accessibility, add /usr/libexec/sshd-keygen-wrapper."
verify = { command = ["screencapture", "-x", "/tmp/hev-node-check.png"], context = "ssh" }
```

`mac.example.test.node.toml`:

```toml
schema = "hev.node/v1"

[node]
name = "mac.example.test"
os = "macos"
arch = "arm64"
account = "agent"
label_prefix = "test.example"
modules = ["modules/base.toml", "modules/harness.toml", "modules/gui.toml"]

[packages.go]
brew = "go"
check = ["go", "version"]

[packages.colima]
brew = "colima"

[packages.docker]
brew = "docker"

[identity]
git_name = "Example Bot"
git_email = "bot@example.test"
github = { login = "example-bot", token = "op://example/bot/token" }
https_only = true

[repos.team-config]
path = "~/workspace/team-config"
url = "https://github.com/example-org/team-config.git"

[repos.widget]
path = "~/workspace/widget"
url = "https://github.com/example-org/widget.git"
admit = true

[files.gitconfig]
path = "~/.gitconfig"
source = "files/mac/gitconfig"
mode = "link"

[files.zshenv]
path = "~/.zshenv"
source = "files/mac/zshenv"
mode = "link"

[files.claude-settings]
path = "~/.claude/settings.json"
source = "files/mac/claude-settings.json"
mode = "link"

[services.runtime]
command = ["/bin/bash", "~/workspace/team-config/bin/runtime-start.sh"]
every = "15m"
run_at_load = true
log = "~/.local/state/hev/node/logs/runtime.log"

[services.disk-prune]
command = ["/bin/bash", "~/workspace/team-config/bin/disk-prune.sh"]
every = "1h"

[services.weekly-report]
command = ["~/.local/bin/report-job", "--weekly"]
working_directory = "~/workspace/widget"
calendar = [{ weekday = 5, hour = 15, minute = 0 }]
secrets = { REPORT_TOKEN = "op://example/reports/token" }
limits = { ceiling = { count = 1, window = "6d" } }

[services.heartbeat]
command = ["~/.local/bin/heartbeat"]
every = "5m"
secrets = { HEARTBEAT_URL = "op://example/heartbeat/url" }
state = "disabled"

[skills]
role = "host"
machine = "desktop"

[dispatch]
host = true
cap = 3
workspace = "~/workspace"

[dispatch.admission]
add_from = ["https://github.com/example-org/"]

[settings.macos]
sleep = false
autorestart = true
remote_login = true

[checks.docker]
command = ["docker", "info"]
timeout = "15s"
```

### Example: a Windows node

`win.example.test.node.toml`, using the same `base` and `harness` modules
(`base` drops `tmux` on Windows through its `os` filter; `gui` is macOS-only
and is not included):

```toml
schema = "hev.node/v1"

[node]
name = "win.example.test"
os = "windows"
arch = "amd64"
account = "builder"
label_prefix = "test.example"
modules = ["modules/base.toml", "modules/harness.toml"]

[packages.go]
winget = "GoLang.Go"
check = ["go", "version"]

[packages.powershell]
winget = "Microsoft.PowerShell"
check = ["pwsh", "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"]

[packages.chrome]
winget = "Google.Chrome"

[identity]
git_name = "Example Bot"
git_email = "bot@example.test"
github = { login = "example-bot", token = "op://example/bot/token" }
https_only = true

[repos.widget]
path = "~/workspace/widget"
url = "https://github.com/example-org/widget.git"
admit = true

[files.gitconfig]
path = "~/.gitconfig"
source = "files/windows/gitconfig"
mode = "copy"

[files.ssh-config]
path = "~/.ssh/config"
source = "files/windows/ssh_config"
mode = "copy"
perm = "0600"           # applied as a protected DACL for the account

[services.nightly-build]
command = ["~/.local/bin/hev.exe", "dispatch", "sweep"]
calendar = [{ hour = 2, minute = 30 }]
log = "~/.local/state/hev/node/logs/sweep.log"

[skills]
role = "host"
machine = "windows"

[dispatch]
host = true
cap = 2
classes = { heavy = 1 }
workspace = "~/workspace"

[settings.windows]
openssh_server = true
sleep = false

[human.elevated-first-run]
do = "Run hev nodes apply once from an elevated terminal as the node account, so the startup triggers can be registered."
verify = { command = ["schtasks.exe", "/Query", "/TN", "\\test.example\\nightly-build"] }
before = ["services"]

[checks.disk]
disk_free = { path = "~", min_gib = 40 }

[checks.git-credential]
command = ["git", "ls-remote", "https://github.com/example-org/widget.git", "HEAD"]
context = "ssh"
timeout = "30s"
```

On Windows the services become Task Scheduler tasks in the
`\test.example\` folder with S4U logon, as kit's daemon task is in `\hev\`
(`cmd/hev/task.go`). `capture` keeps using kit's own task. Admin accounts
use `administrators_authorized_keys` with its ACL, chosen by the same
`--administrators` logic `hev nodes accept` has (`cmd/hev/nodes.go`). The
`context = "ssh"` check on the credential exercises the case `docs/nodes.md`
warns about: credential helpers that need a desktop fail inside an SSH
session.

## Phases

**Phase 0** is this RFC.

**Phase 1: schema, plan, doctor.** Loader and composition with provenance; a
JSON Schema under `schemas/` for editors; observation drivers for all three
platforms; `plan`, `doctor` and `--host`. No code path in phase 1 changes the
machine. Accepted when manifests written for the operator's existing macOS and
Linux hosts produce no drift on `doctor` and no actions on `plan`, against the
live hosts, without changing them. A Windows manifest is validated against a
Windows CI runner and, when one exists, a real Windows node.

**Phase 2: apply and bootstrap.** Package backends, generalised service and
schedule renderers, files with receipts and rollback, repositories, identity,
skills, dispatch keys, settings, human-step gating and `provide`, attempt
limits, `bootstrap` with seeded enrollment and the owner grant. Accepted by
standing up a new node from a manifest alone: a fresh Linux VM from the
first-boot script and a fresh Windows VM from the PowerShell script, each to a
green doctor with only the declared human steps performed. Cutover of an
existing host is a separate, scheduled act after this.

**Later.** A provider wrapper that creates VMs and passes the script; a Flox
backend for macOS and Linux; more Linux package managers; secret-bearing
files (open question 3); a fleet view (`hev nodes doctor --all`).

## Open questions

1. **Module conflicts: error or last-wins?** Two modules setting the same key
   differently could fail, or the later module could win. Recommendation:
   fail, and let the node file override. Order-independent results are worth
   the occasional explicit override.
2. **How does a node get its manifest after first boot?** Options: the owner
   pushes it each time (`--host`), or the node clones the private repository
   that holds it, as one of its `repos`, and applies from there.
   Recommendation: support both, and make the declared repository the normal
   path, so a node can be re-applied from its own checkout without the
   owner's machine.
3. **Files that must contain a secret.** Some third-party daemons read their
   API key only from a config file. Options: never support it (the operator
   keeps a private script for that daemon), or add a `secret_file` resource
   that renders references at apply time into a 0600 file and keeps only
   digests in receipts. Recommendation: leave it out of v1 and add
   `secret_file` in a later phase behind an explicit per-resource flag, since
   it breaks the "kit never writes a secret value to disk" line RFC 0009
   draws.
4. **Attempt limits in kit or private?** Cooldowns and rolling ceilings on
   scheduled jobs protect rate-limited external posting. Recommendation:
   include `limits` in phase 2; it is small, generic, and the alternative is
   every operator keeping a wrapper script.
5. **What grant does a bootstrapped node give its owner?** An unrestricted
   key line (owner grant) lets `plan`, `apply --host` and `provide` work over
   SSH; a forced-command key would need new RPC actions for each.
   Recommendation: an owner grant, shown as `owner` in `hev nodes ls` so it
   is visibly different from a teammate's grant, and revocable with
   `hev nodes remove`.
6. **Should `hev nodes ls` show machines?** Today it lists grants, joins and
   invitations. Recommendation: keep `ls` as it is and show manifest state in
   `doctor` (and later `doctor --all`), so access and configuration stay
   separate.
7. **Version pinning.** Recommendation: names only by default, with optional
   `version` for backends that honour it, and `--upgrade` to move a pin.
   Floating everything on each apply makes runs unrepeatable; pinning
   everything makes manifests stale.
8. **Windows services: Task Scheduler or the Service Control Manager?**
   Recommendation: Task Scheduler with S4U for user services, as kit's daemon
   already does; it needs no stored password and matches the per-user model
   on macOS and Linux. System services stay out of v1.
9. **Linux beyond apt.** Recommendation: apt only in v1, behind a backend
   interface so pacman or dnf can follow when a node needs it.
10. **Per-schedule time zones.** launchd evaluates calendars only in the
    system's time zone. Recommendation: calendars use the node's local time;
    nodes that need another zone set the system zone. A per-service zone would
    work on systemd and Task Scheduler but not launchd.
