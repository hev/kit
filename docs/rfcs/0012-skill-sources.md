# RFC 0012: Team skill sources

Status: proposed

## Problem and decision

Bundled skills travel with the CLI. A team's own skills need the same path to
sender laptops and dispatch hosts, with reproducible installs and retirement.
Add `skills.sources` to the existing config (`HEV_CONFIG`, otherwise
`~/.hev/config.toml`). Keep bundled installation and its edit protection.

```toml
[skills]
machine = "worker" # explicit team-defined label; no hostname inference
role = "host"      # sender or host; omitted selects the root half

[[skills.sources]]
url = "https://example.com/team/skills.git"
ref = "refs/heads/main"
subdir = "skills"

# Alternatively:
# [[skills.sources]]
# path = "../team-skills" # relative to config, not the process cwd
# subdir = "skills"
```

Set exactly one URL or path per source. Ref defaults to HEAD; use a full branch
ref for remote sources. Git paths install committed content only, even with a
dirty checkout. Plain directories use a SHA-256 content snapshot instead of a
Git commit; they cannot specify a ref. Git is required for repository sources.
A subdir contains one directory per skill, each containing SKILL.md. Source
names may not collide with bundled skill names or another source's installs.

`hev skills sync` fetches URL mirrors, resolves the commit, rejects a rewrite
unless the previous installed commit is its ancestor, and builds a new pinned
`~/.local/opt/hev-skills-<source-id>-<commit>` directory before publishing links.
Git sources never merge, reset, or modify the input checkout. Branch changes
in config deliberately create a new source identity. Plain directory updates
are content revisions and have no ancestry constraint. Pins are retained so a
running process can continue using an older release; garbage collection is
outside this version.

`hev skills check` resolves local sources and cached remote refs without
fetching and reports each source ID, commit and current/drift state. A missing
mirror, changed commit, missing install or retired link returns nonzero. This
is local drift detection, not a network freshness probe. Sync is the scheduled
network update. Unchanged installs do not republish destinations. Fetch and
archive costs remain proportional to the repository; prefer a dedicated small
skills repository. Credentials come from Git's existing SSH/credential setup;
never put tokens in config URLs.

## Targeting and two halves

Install metadata appears in YAML front matter, either at top level or under
`metadata`. Scalars, inline arrays and block arrays are accepted:

```yaml
---
name: example
metadata:
  harnesses: [claude, codex, agents]
  machines: [worker, desktop]
  roles: [host, sender]
---
```

Missing harnesses means claude and codex. `all` means those plus the shared
`~/.agents/skills`. Harness roots are created even if the harness has never run.
Machines and roles are opt-in allowlists; a missing configured label does not
match a restricted skill. Unknown harness names and invalid roles are errors.
A role of host selects `boh/SKILL.md` when present, along with that half's
metadata and bin directory. Sender (or an omitted role) selects the root half.
`boh` is a compatibility directory spelling, not a machine naming convention.
All files in the selected half's `bin/` are exposed in `~/.local/bin`; authors
must give Unix commands executable mode and use platform-appropriate command
extensions on Windows. Add `~/.local/bin` to PATH using the shell's normal
configuration. The installer does not edit shell startup files.

## Ownership and publication

An ownership manifest under `~/.local/state/hev-skills` records destinations,
content digests and resolved commits. Sync preflights all sources and foreign
collisions before changing harness or command paths. Only matching owned
installs are replaced or pruned, including skills removed from a repository,
removed source config, or newly excluded machine/role/harness targets.
Foreign files, directories, links, edited copies and symlinked destination
parents cause a clear error and are left in place. `--force` applies only to
bundled skills, never to foreign source destinations. A source checkout's
symlinks and special files are rejected rather than followed outside its tree.

Unix publishes staged symlinks by rename. Windows stages copies and renames
them, avoiding Developer Mode and administrator requirements. The copy path
checks content digests before replacement or pruning. Each destination is
published as a complete directory/file; there is no global atomic switch
across all three harnesses. An ordinary publication error rolls back all
changed destinations. The manifest is published only after the paths succeed.
A process lock rejects concurrent invocations. An interrupted process can leave
a stale lock or an incomplete publication: inspect the retained state before
removing the lock; no automatic stale-lock takeover is attempted. This is not
a package sandbox: installing a repository trusts its instructions and commands.

## Scheduling

No daemon is added. Run as the same user as the harness, with explicit HOME,
HEV_CONFIG and Git credentials. Log failures and investigate rather than loop
until a run happens to succeed. Do not overlap an install with a manual sync.
For example, a systemd user oneshot service:

```ini
[Service]
Type=oneshot
ExecStart=%h/.local/bin/hev skills sync
```

And its matching `.timer`:

```ini
[Timer]
OnBootSec=5m
OnUnitActiveSec=1h
Persistent=true
[Install]
WantedBy=timers.target
```

On macOS, a user LaunchAgent can run hourly (replace paths):

```xml
<plist version="1.0"><dict>
<key>Label</key><string>com.example.skills-sync</string>
<key>ProgramArguments</key><array>
<string>/Users/USER/.local/bin/hev</string><string>skills</string><string>sync</string>
</array>
<key>StartInterval</key><integer>3600</integer>
<key>StandardErrorPath</key><string>/Users/USER/Library/Logs/skills-sync.log</string>
</dict></plist>
```

On Windows, register a scheduled task as the harness user (PowerShell):

```powershell
$action = New-ScheduledTaskAction -Execute "$env:USERPROFILE\.local\bin\hev.exe" -Argument 'skills sync'
$trigger = New-ScheduledTaskTrigger -Daily -At '09:00'
$settings = New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew
Register-ScheduledTask -TaskName 'Team skill sync' -Action $action -Trigger $trigger -Settings $settings
```

## MCP gap and later work

Skills can describe tools but this installer does not distribute or configure
MCP servers. That gap includes binaries/runtimes, dependency versions,
per-harness configuration schemas, credentials, consent, process lifetime and
health checks. A later proposal could add declarative server references with
platform artifacts, checksums and per-harness adapters, separating secret
references from committed configuration and requiring explicit enablement.
It should not execute arbitrary install scripts during skill sync.
