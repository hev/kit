# `hev peers` parity with the Python `peers`

`hev peers` (and a `peers` symlink to `hev`) now runs `internal/peers/compat`,
a function-by-function port of the Python CLI. It reads and writes the same
files under `~/.local/state/peers-v2`. It produces the same JSON bytes (key
order, `json.dumps` separators, `ensure_ascii`, Python float `repr`), the same
command output and the same wire formats. The Python and Go versions can
therefore run side by side on one host and across hosts during the switch.

Spec: `hev/skills` `peers/` at `947e74f` (2026-11-04). Oven's installed snapshot
(`~/.local/opt/factory-skills-a2b71685d2cb/skills/peers`) differs from it only in
one usage line. The Python source stays outside this repository; the interop
tests take its path from `PEERS_PYTHON_REFERENCE`.

The native engine from #165 (`internal/peers`) stays for the callers that rely
on its contracts: dispatch notifications (`HEV_PEERS_ENGINE=native`, set by
`executeNotification`), `--inbox-only`, the SSH `hev dispatch rpc` peers
action, the HTTP listener in `hev daemon`, `hev overview` and `hev expedite`'s
sender resolution. `HEV_PEERS_BACKEND`, when set, still forwards `hev peers` to
an external program; the `peers` basename ignores it, as before.

## Commands and flags

"Implemented" means the Go port does what the Python does and prints the same
output. Where the interop suite compares both implementations byte for byte,
the row names the test. A deviation is described where it exists.

| Command / flag | Python behaviour | `hev peers` |
| --- | --- | --- |
| `peers` (no args), `peers -FLAG…` | runs `ls` with those flags | Implemented |
| `-h`, `--help`, `help` | prints the module docstring | Implemented. The text is the docstring plus two lines for `inbox-hook` and `runtime`; a test checks that nothing else differs. `hev peers --help` prints it under cobra's usage header |
| `--from SESSION` (any position) | sets `PEERS_FROM`: the sender, resolved by `find`, never receipt ownership; a managed sender also needs real ancestry | Implemented |
| unknown command | `peers: unknown command 'x'; peers --help`, exit 1 | Implemented (`TestInteropWaitPasteAndErrors`) |
| `ls` | rows from tmux panes, managed runners and headless orphans. Prints address, harness, state, cwd [branch], title, ROLE, kind, inbox counts, attached/over ssh, activity age, `← sender Nm ago` notes and `← you`. Snapshot hosts follow | Implemented (`TestInteropInboxConsumeAck`, `TestInteropBeaconDelivery`) |
| `ls -a` / `--all` | includes plain shells | Implemented |
| `ls --everywhere` / `-e` | adds live `ls` from each `$PEERS_HOSTS` over ssh; an unreachable host prints `HOST/: unreachable (…)` | Implemented (`TestInteropRemoteSSH`) |
| `ls --json` | version 1 inventory: per-pane identity, state, branch, role, incarnation, `inbox_status`; `hosts[]` from ssh and from snapshots (with `stale`) | Implemented; byte-identical apart from `at` |
| `read TARGET [-n N]` | the last N lines (default 60) of a tmux capture. Managed runners show their checkpoint. `pid:N` renders the Claude transcript. `d-NAME` renders `hev dispatch logs` | Implemented. The transcript lookup uses `statx` birth time on Linux, where the Python's `st_birthtime` raises |
| `read HOST/TARGET` | ssh `read`; a snapshot-only host prints the pushed screen plus `[HOST/x as of …]` | Implemented (`TestInteropRemoteSSH`, `TestInteropBeaconDelivery`) |
| `send TARGET MESSAGE\|-` | inbox-on session: held in the bound inbox. Plain idle pane: bracketed paste and Enter. `d-NAME`: `hev dispatch say`. Refuses own pane, plain shell, headless run, PI without inbox, offline role, prompt; draft and `?` without `--force` | Implemented (`TestInteropWaitPasteAndErrors`, all send paths) |
| `send --force` | overrides the draft and unknown-screen checks only | Implemented |
| `send --id/--order/--run` | stable identifiers `[A-Za-z0-9._:-]{1,160}`. A retry with the same content dedupes; changed content is refused | Implemented. A cross-implementation retry dedupes (`TestInteropInboxConsumeAck`) |
| sender resolution | `PEERS_FROM`, then a managed runner above the process, `$TMUX_PANE`, the agent pane in its ancestry, the pane PID in its ancestry; with none, refuse with the `--from` hint | Implemented |
| bare-name relay alias | an unknown bare name resolves to the one snapshot host that has it; several hosts → refuse | Implemented (`TestInteropBeaconDelivery`) |
| `send HOST/TARGET` | ssh to a `$PEERS_HOSTS` host; otherwise queue in the outbox against a snapshot (`[inbox ID host:id]`) | Implemented. Deviation in transport, see below |
| `wait TARGET [--timeout S]` | polls every 3 s until not `working`. `pid:N` gone prints `finished`. Exit 2 on timeout. Snapshot hosts poll every 15 s | Implemented. When the target is missing, Python crashes with a traceback; Go prints `peers: no pane matches …` and exits 1 |
| `role [SESSION] [foh\|boh\|offline]` | inspects or sets the tmux `@peers-role`. Remote targets and headless/dispatch rows are refused | Implemented (`TestInteropRoleAndOffline`) |
| `inbox` (plain) | prints unread messages and consumes them (recipient only) | Implemented (`TestInteropInboxConsumeAck`) |
| `inbox --peek` | inspects without consuming | Implemented |
| `inbox --count` | number of unread | Implemented |
| `inbox --json` | `{session, recipient_incarnation, on, unread, unacknowledged, historical, messages}` | Implemented; byte-identical |
| `inbox --all` | all retained history, current and historical, not deduped | Implemented |
| `inbox --current-all` | all of the current incarnation | Implemented |
| `inbox --pending` | consumed-but-unacknowledged replay | Implemented |
| `inbox --limit N` | 1..64, otherwise `--limit must be 1..64` | Implemented |
| `inbox --session S` | another local session (inspection, or setup for `on/off`) | Implemented |
| `inbox --session HOST/S` | ssh inspection only; any mutating op → refuse. A non-ssh host → "ACK status unavailable" | Implemented (`TestInteropRemoteSSH`) |
| `inbox on\|off [SESSION]` | sets `@peers-inbox`; a managed name is refused | Implemented |
| `inbox bind-runtime PID` | the launcher (in this process's ancestry, interactive inbox recipient) gets a 64-hex token in `runtime/TOKEN.json` | Implemented (`TestInteropRuntimeBinding`) |
| `inbox consume ID…` | validates every ID first, then appends `consumed` receipts | Implemented |
| `inbox ack ID…` | needs prior consumption; appends `acknowledged` receipts | Implemented |
| `inbox owner` | `{session, recipient_incarnation, on}`; recipient only | Implemented |
| `inbox hold [--sender X] -` | holds stdin under `[held message from X]` | Implemented |
| `PEERS_INBOX_TOKEN` | receipt ownership through a live launcher binding; no fallback | Implemented. A dead launcher's token is refused by both (`TestInteropRuntimeBinding`) |
| `managed register NAME --root ABS [--replace]` | binds one live `runner.py ROOT` lifetime. Idempotent for the same lifetime; `--replace` archives to `.history.jsonl` | Implemented (`TestInteropManagedRegister`) |
| managed rows | `managed:NAME` rows, checkpoint state, inbox always on, sender needs runner ancestry | Implemented |
| `beacon` | builds a snapshot (screens, links, recent sends, relay outcomes). Ships it and the sent-log tail (byte cursor) to each `$PEERS_HOSTS` via `_recv`, stores the fetched relays durably, then delivers pending | Implemented (`TestInteropBeaconDelivery`, three directions) |
| `_recv` (wire) | stores the snapshot and sent log, returns the outbox for that host. With `transfer_protocol: 1`, transferred IDs are deleted | Implemented |
| `_chat` (wire) | delivers a chat payload to local members and returns `[[addr, outcome]]` | Implemented (`TestInteropRemoteSSH`) |
| `_chatlog ID` (wire) | this host's records for a thread | Implemented |
| `chat new MEMBER… [--title T] [-m MSG\|-]` | thread with incarnation-bound members; optional first message | Implemented. Deviation: Python sends a literal `-` for `-m -`; Go reads stdin as the usage says |
| `chat say ID MESSAGE\|-` | fans out locally, over `_chat`, or through the relay outbox; re-binds the sender if needed | Implemented (`TestInteropChat`) |
| `chat add ID MEMBER… [-m MSG\|-]` | binds the new members; default text includes the last 5 messages | Implemented |
| `chat leave ID [--reason T]` | notifies the others and marks the sender left | Implemented |
| `chat leave --all [--reason T] [--hook HARNESS] [-q]` | the `/clear` hook form; `--hook` reads hook JSON on stdin and picks the single pane of that harness in the hook's cwd | Implemented (`TestInteropChat`) |
| `chat ls`, `chat show ID [-n N]` | known threads; folded transcript including remote `_chatlog` | Implemented; byte-identical |
| `chat`, `chat -h/--help/help` | prints the chat usage | Implemented |
| `chef-notify` | retired diagnostic, exit 1 | Implemented |
| v2 gate (`cutover-ready.json` + sealed `~/.local/state/peers`) | refuses unless the reviewed cutover completed | Implemented, with one deviation: a host with no legacy `~/.local/state/peers` at all (a fresh install) may run |
| `peers-inbox-hook` | UserPromptSubmit adapter: `inbox --pending --limit 8 --json`, then `additionalContext` with the ACK instruction, or the "FOH inbox held" system message | Implemented as `hev peers inbox-hook`; the `peers-inbox-hook` basename also works. Output byte-identical (`TestInteropInboxHook`) |
| `peers-runtime codex\|claude\|pi [ARGS]` | binds a token, then launches the harness with an invocation-only UserPromptSubmit hook. `PEERS_RUNTIME_LAUNCHER` wraps the harness. Forwards SIGTERM/SIGHUP | Implemented as `hev peers runtime`; the `peers-runtime` basename also works. The hook command is this executable's `inbox-hook` (absolute path) rather than `peers-inbox-hook` on PATH, so binder and hook cannot drift |
| `peers-history` | rebuilds peer exchanges from transcripts (offline forensics) | **Dropped.** It never touches peers state or the wire, so there is no parity risk. Keep running the Python script where it is wanted |
| `peers-cutover` | one-time v1→v2 seal and readiness marker | **Dropped.** Every fleet host has already run it, and the Go gate honours its marker. A new host has no v1 state to migrate |
| `lib/durable.py` (`PEERS_TRANSPORT=sql`) | inactive SQL transport; nothing selects it | **Dropped.** It is not used by `bin/peers` |
| `mod/` (Claude plugin), `pi/` (PI extension) | TypeScript consumers of `inbox --pending/consume/ack` | Not ported: they are harness plugins, not CLI. They call `peers inbox …`, whose JSON and exit codes are identical, so they should work unchanged. The suite does not exercise them |

### Environment

| Variable | Status |
| --- | --- |
| `PEERS_SELF` | Implemented. The default label is `hostname -s` lowercased, with `*mac-mini*` → `mini` and `*macbook*` → `macbook`, exactly as in the Python, so incarnation hashes match without setting it |
| `PEERS_HOSTS` | Implemented (default `mini oven`, minus this host) |
| `PEERS_FROM`, `PEERS_NO_SELF`, `PEERS_AGENT`, `TMUX_PANE`, `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CODEX_*` | Implemented |
| `PEERS_HOST_LABEL`, `PEERS_REMOTE_FROM/AGENT/CWD/REPLYABLE`, `PEERS_STDIN_B64` | Implemented: set on the remote side of ssh, and honoured when received |
| `PEERS_INBOX_TOKEN`, `PEERS_INBOX_BIN`, `PEERS_RUNTIME_LAUNCHER` | Implemented |
| `PEERS_REMOTE_PEERS` | New: the remote command Go runs over ssh (default `peers`) |
| `HEV_PEERS_STATE` | hev-only override (unset on every fleet host). Both the native engine and the compat CLI use it in place of `$HOME/.local/state/peers-v2`, so dispatch notifications and `hev peers inbox` share one tree; the Python ignores it |

## Storage (all under `~/.local/state/peers-v2`)

Every path is read and written with the Python's format, locking (`flock` on
the same lock files) and durability (fsync where the Python fsyncs).

| Path | Writer semantics | Status |
| --- | --- | --- |
| `inbox/SESSION.jsonl`, `.lock`, `.receipts.jsonl` | locked append, torn-tail repair, fsync; receipts keyed by (incarnation, id) | Implemented; same field order (tested) |
| `inbox/SESSION.cursor` | legacy, never written by v2 | Untouched by both |
| `sent.jsonl` | unlocked append of every delivery and chat `say` | Implemented |
| `hosts/HOST.json`, `hosts/HOST.sent.jsonl` | snapshot (atomic replace) and shipped sent log | Implemented |
| `sent-cursor-HOST` | byte offset into `sent.jsonl`, reset if the log shrank | Implemented (reship test) |
| `outbox/SHA.json`, `queued.jsonl` + `.lock` | relay queue keyed by sha256(from_host:id), dedupe by fingerprint | Implemented |
| `pending/SHA.json`, `relay-transfers.jsonl` + `.lock` | durable fetch receipt, then completion; refetch never recreates | Implemented (shared across alternating Go/Python beacons in tests) |
| `relayed.jsonl` | relay outcomes (held/submitted/waiting/dropped) | Implemented |
| `threads/ID.json` | thread record, merged per member (latest join/leave, binding) | Implemented; same key order (tested) |
| `managed/NAME.json`, `.history.jsonl`, `.register.lock` | 0600 registration | Implemented |
| `runtime/TOKEN.json` | 0600 O_EXCL launcher delegation | Implemented |
| `cutover-ready.json`, legacy seal `~/.local/state/peers` | read-only gate | Implemented (see deviation) |

Files are not just compatible but byte-for-byte identical. The Go writer uses
an insertion-ordered JSON model with Python's `json.dumps` separators,
`ensure_ascii` escaping (including `\u007f` and surrogate pairs) and float
`repr`. Unit tests in `compat_test.go` pin these against CPython output.

## Deliberate deviations

1. **Remote transport.** Python runs `python3 -c <its own source>` on the remote
   host. A compiled binary cannot ship itself, so Go runs the remote host's
   installed `peers` (`$PEERS_REMOTE_PEERS`, with `~/.local/bin` first on PATH).
   It sends the same environment, argv and raw stdin. Either implementation can
   answer, but a host reached by Go must have a `peers` installed (every fleet
   host does). Python callers are unaffected: they still ship their own source.
2. **Crashes become errors.** A missing `wait` target, a missing `lsof` or
   `st_birthtime` on Linux, and `int()` failures make the Python exit with a
   traceback. Go prints `peers: …` and exits 1. Exit codes match.
3. **`chat new -m -`** reads stdin (the documented behaviour) instead of sending
   a literal `-`.
4. **Fresh hosts** without any legacy `~/.local/state/peers` pass the v2 gate.
5. **`runtime` hook wiring** names this executable's `inbox-hook` directly.
6. **Help** has two extra lines for the folded-in helpers.

## Interop tests

`internal/peers/compat/interop_unix_test.go` builds a scratch cluster for each
test. Every simulated host gets its own `HOME` (with a completed-cutover
marker), its own tmux server (`TMUX_TMPDIR`), `PEERS_SELF` and `PEERS_HOSTS`. A
fake `ssh` on PATH runs a remote command as the target host, and the mini cannot
reach the laptop. Agent panes are `exec -a claude sleep` processes, so both
implementations classify them as Claude. Most checks run one command through
both implementations and require identical stdout, stderr and exit status.

| Test | Directions |
| --- | --- |
| `TestInteropInboxConsumeAck` | Python send → Go read/consume → Python ACK; Go send → Python consume → Go ACK; same row/receipt field order; cross-implementation `--id` retry dedupe and conflict; hold both ways; `--limit`, `owner`, `--current-all`, `--all`, `ls`, `ls -a`, `ls --json`, `role` byte-identical |
| `TestInteropRoleAndOffline` | Python sets offline, Go refuses delivery; Go sets FOH; `inbox on/off` |
| `TestInteropChat` | Python `chat new` → Go `chat say` → Python reads; Python `chat add` backlog → Go reads; Go leave, Python `/clear` hook leave; `chat show/ls` byte-identical; Go-started chat read by Python |
| `TestInteropBeaconDelivery` | three sub-tests: Go beacon + Python sender, Python beacon + Go sender, Go + Go. Snapshot push, bare-name relay alias, idempotent queue, snapshot read, fetch → durable pending → inbox delivery read/ACKed by the other implementation, transfer completion clearing the remote outbox, relay outcome in the next snapshot, sent-log cursor not reshipping, relayed chat, relayed paste |
| `TestInteropRemoteSSH` | Go → Python install over ssh and Python → its shipped source; `ls --everywhere` (text and JSON), remote inbox inspection, remote read, cross-host chat over `_chat`/`_chatlog`, identical unreachable-host error |
| `TestInteropInboxHook` | Python `peers-inbox-hook` vs `hev peers inbox-hook`: identical output, a single consumption, identical "held" fallback, silence for other events |
| `TestInteropWaitPasteAndErrors` | `wait`, direct paste delivery, 16 refusal/usage paths byte-identical, help text |
| `TestInteropManagedRegister` | Go registers a live runner; idempotent re-register, duplicate-lifetime refusal, `read`, `ls` byte-identical; Python sends to the managed inbox |
| `TestInteropRuntimeBinding` | `hev peers runtime claude` binds a token. The launched harness consumes with the Python under that token and replays with Go. A dead launcher's token is refused by both |

Run them with:

```sh
git clone https://github.com/hev/skills /tmp/skills   # private
PEERS_PYTHON_REFERENCE=/tmp/skills/peers/bin/peers go test -count=1 ./internal/peers/compat/
```

Against `947e74f` every test passes, including three consecutive `-race` runs.
Against Oven's installed snapshot `a2b71685`, every test passes except the help
comparison in `TestInteropWaitPasteAndErrors`. That snapshot's docstring has one
extra usage line (`new tmux session running an agent, optional first prompt`)
for a command it no longer has.

Without `PEERS_PYTHON_REFERENCE` the interop tests skip. The unit tests
(JSON/repr/shlex goldens, classification, addressing, fingerprints) always run.
A green default run is not interop evidence.

## Cutover plan

Do not flip any host as part of this change. A host flips when its operator
runs the steps below. Order: Oven first (it only receives beacons and runs
dispatch workers), then the mini, then the laptop (the beacon host). The formats
are identical, so mixed hosts interoperate at every step.

Before any host: install the reviewed `hev` build (`~/.local/bin/hev`, or the
release directory the host already uses). Run
`PEERS_PYTHON_REFERENCE=… go test ./internal/peers/compat/` on that platform
(macOS for the laptop and the mini). Record the current link targets for
rollback:

```sh
for f in peers peers-inbox-hook peers-runtime; do readlink ~/.local/bin/$f; done > ~/peers-rollback.txt
```

### Oven (Linux)

| What | Today | Flip to |
| --- | --- | --- |
| `~/.local/bin/peers` | → `~/.local/opt/factory-skills-a2b71685d2cb/skills/peers/bin/peers` | `ln -sfn ~/.local/bin/hev ~/.local/bin/peers` |
| `~/.local/bin/peers-inbox-hook` | → the same snapshot's `bin/peers-inbox-hook` | `ln -sfn ~/.local/bin/hev ~/.local/bin/peers-inbox-hook` |
| `~/.local/bin/peers-runtime` | → the same snapshot's `bin/peers-runtime` | `ln -sfn ~/.local/bin/hev ~/.local/bin/peers-runtime` |
| `HEV_PEERS_BACKEND` | set to `~/.local/libexec/kitchen/peers` (a Python copy) in dispatch launch environments | remove it from wherever the launch environment sets it, so `hev peers` runs native |
| `peers-history`, `peers-cutover` | snapshot links | leave as they are (dropped, not replaced) |
| skill installer | reinstalling the factory-skills snapshot repoints `~/.local/bin` links | do not reinstall the snapshot after the flip, or drop these three links from it first |
| beacon | none: Oven receives `_recv` over ssh, no timer | nothing to change. After the flip, the laptop's beacon lands on Go's `_recv` |
| `~/.claude/settings.json` | no peers hooks | nothing |
| `~/.config/hev/dispatch.toml` `notify = ["peers", "send", …]` | resolves to the Python | resolves to hev with no edit |

### Mini (macOS)

| What | Today | Flip to |
| --- | --- | --- |
| `~/.local/bin/peers`, `peers-inbox-hook`, `peers-runtime` | hev/skills `install.sh` links → `~/skills/peers/bin/*` | `ln -sfn "$(command -v hev)" ~/.local/bin/<name>` for each. Rerunning `skills/install.sh` repoints them, so either first remove `peers/bin/{peers,peers-inbox-hook,peers-runtime}` from hev/skills in the same change, or teach `install.sh` to skip them |
| Claude `SessionStart` (matcher `clear`) in `~/.claude/settings.json` (from hev/shell `claude/settings.json`) | `~/.local/bin/peers chat leave --all --hook claude -q --reason '…'` | no edit: the path now resolves to hev. Optional: `hev peers chat leave …` |
| Codex `SessionStart` in `~/.codex/hooks.json` (hev/shell `codex/hooks.json`) | `~/.local/bin/peers chat leave --all --hook codex -q …` | no edit |
| UserPromptSubmit | injected per launch by `peers-runtime` (`--settings` / `-c hooks…`) | new launches wire `hev … inbox-hook` automatically; running sessions keep their launch-time hook until restart. Mixed is safe: the same receipts |
| beacon | none (the mini cannot reach the laptop) | nothing |

### Laptop (macOS, the beacon host)

Same three links and hooks as the mini. In addition:

| What | Today | Flip to |
| --- | --- | --- |
| `~/Library/LaunchAgents/com.hev.peers-beacon.plist` (hev/shell `launchd/`) | `ProgramArguments = [/Users/hev/.local/bin/peers, beacon]`, every 60 s, `PEERS_HOSTS="mini oven"` | no edit needed: the path is the flipped symlink. To make it explicit, change it to `[/Users/hev/.local/bin/hev, peers, beacon]`, then `launchctl bootout gui/$UID/com.hev.peers-beacon && launchctl bootstrap gui/$UID ~/Library/LaunchAgents/com.hev.peers-beacon.plist` |
| remote side of the beacon | the laptop's Python ships its own `_recv` to the mini and Oven | the laptop's Go runs the remote's installed `peers _recv`. Both remotes must have a `peers` on PATH (they do, Python or Go) |

Verify on each host right after its flip, with no live consumers touched:

```sh
peers --help | head -3
peers ls && peers ls --json | head
peers inbox --peek --pending --json      # from an inbox pane: same as before the flip
peers send <a scratch session> "cutover check" && peers chat ls
tail -n 3 /tmp/peers-beacon.log         # laptop: no errors on the next minute
```

### Rollback

Per host, restore the recorded link targets (`ln -sfn "$target" ~/.local/bin/$f`
for each line in `~/peers-rollback.txt`). Put `HEV_PEERS_BACKEND` back on Oven
if it was removed, and on the laptop revert the plist if it was edited and
re-bootstrap the agent. No state migration is needed in either direction: both
implementations read and write the same files, so messages, receipts, threads,
snapshots, queues and cursors stay valid. Never delete receipts, reset cursors
or touch the legacy seal during a rollback. Sessions launched by the Go
`runtime` keep the Go hook until they restart. That is safe after rollback,
because the hook only calls `inbox --pending --json` with the same token
semantics.
