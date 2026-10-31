# Windows transcript capture

Use the Windows amd64 `hev.exe` in a permanent directory on your user PATH,
for example `%USERPROFILE%\bin`. A source build is also supported:

```powershell
go build -o hev.exe ./cmd/hev
.\hev.exe init
```

Run `hev init` as the Windows user whose transcripts should be captured. Set
an authorized Layer endpoint, scoped API key, and namespace (for example
`windows-traces`). Capture and search use the same `[layer]` configuration.
`LAYER_NAMESPACE`, when set, overrides the file namespace. Put persistent
settings in the file: scheduled tasks do not inherit later shell overrides.
The default config is `%USERPROFILE%\.hev\config.toml`; `HEV_CONFIG` can
select a different file at installation time. The task records that absolute
config path, so changing `HEV_CONFIG` later requires reinstalling the task.

Capture scans the current user's transcript trees recursively:

- Codex: `%USERPROFILE%\.codex\sessions`
- Claude Code: `%USERPROFILE%\.claude\projects`

No transcript hooks are required. Missing transcript trees are harmless.

## Task Scheduler lifecycle

```powershell
hev daemon install
hev daemon status
hev daemon stop
hev daemon start
hev daemon uninstall
```

Installation registers and starts the per-user `\hev\hevd` Task Scheduler
task, using the executable's current absolute path. Keep that executable in
place. The task uses S4U logon without storing a password, with startup and logon
triggers that repeat every minute. Registering the startup trigger may require
an elevated terminal for the same user. Reboot startup still needs native
verification. `start` enables and runs the installed task and confirms daemon startup; `stop`
disables the task and waits for shutdown. Stop and uninstall support repeated calls. Uninstall
removes the task while preserving configuration, logs and capture state.
`hev daemon` runs capture in the foreground for diagnosis.

State lives under `%USERPROFILE%\.hev`: `daemon.pid` holds process identity,
`daemon-status.json` records the last scan, `index-state.json` tracks successful
source signatures, and `hevd.out` / `hevd.err` contain scheduled process output.
Archive migration journals and redaction state live beside the selected config.
Private files use Windows ACLs restricted to the user, SYSTEM and Administrators.

## Verify capture

Create a transcript containing a distinctive, nonsecret phrase, then allow a scan
(default interval: five minutes). Use the same configured namespace to inspect it:

```powershell
hev s
hev traces ls
hev traces search "distinctive capture verification phrase"
```

`hev s` should report a running daemon, a recent scan, and no last error. Inspect
`hevd.err` if capture is unhealthy. Listing confirms session ingestion; search
confirms the phrase is retrievable. `hev index --dry-run --limit 5` checks local
parsing without uploading. Hosted capture/search with a scoped credential still
requires verification on a native Windows host; cross-builds alone do not prove it.
