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
an external program, except for calls that must stay in this implementation.
The `peers`, `peers-inbox-hook` and `peers-runtime` basenames never forward,
and neither do `hev peers inbox-hook` and `hev peers runtime`, which only this
implementation provides. When the hook and the runtime call back into `hev`
(for the runtime's `inbox bind-runtime`, the hook's inbox read, and the
`inbox ack` command the hook tells the agent to run), they use the hidden
`hev peers-native` entry. That entry ignores the backend too, so an inherited
`HEV_PEERS_BACKEND` cannot split binder, hook and ACK across two stores.

## Commands and flags

"Implemented" means the Go port does what the Python does and prints the same
output. Where the interop suite compares both implementations byte for byte,
the row names the test. A deviation is described where it exists.

| Command / flag | Python behaviour | `hev peers` |
| --- | --- | --- |
| `peers` (no args), `peers -FLAG…` | runs `ls` with those flags | Implemented |
| `-h`, `--help`, `help` | prints the module docstring | Implemented. The text is the docstring plus lines for `inbox-hook`, `inbox --wait` and `runtime`; a test checks that nothing else differs. `hev peers --help` prints it under cobra's usage header |
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
| `beacon` | builds a snapshot (screens, links, recent sends, relay outcomes). Ships it and the sent-log tail (byte cursor) to each `$PEERS_HOSTS` via `_recv`, stores the fetched relays durably, then delivers pending | Implemented (`TestInteropBeaconDelivery`, three directions). Adds a per-host ssh deadline (deviation 9, `TestSSHDeadlineBoundsAStuckHost`) |
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
| — (no reference command) | — | `hev peers inbox --wait [--timeout D]`: a Claude Stop/StopFailure asyncRewake hook that exits 2 with new bound-inbox items, never consuming or acknowledging them. Never forwarded to `HEV_PEERS_BACKEND`. See [peers.md](peers.md#waking-an-idle-claude-session) (`TestInboxWait*`) |
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

Lone surrogates survive. Python keeps a lone `"\udcff"` (for example a
surrogateescape byte from a non-UTF-8 argv or path) and writes it back
unchanged. The Go decoder therefore parses JSON itself, as `encoding/json`
would turn the escape into U+FFFD. It keeps a lone surrogate as its WTF-8
bytes, and the encoder writes those bytes back as the same `\uXXXX`. A byte
that is not UTF-8 (one Go got from the OS, such as raw argv) is written as
surrogateescape's `\udcXX`, which is what Python writes for the same byte.
Reading and rewriting therefore keeps every Python string. One input can still
disagree: OS bytes that happen to form a WTF-8 surrogate (`ED A0..BF 80..BF`)
come out as one escape, while Python writes three `\udcXX`.

Text transforms work on the same WTF-8 form. Python's `s[:n]` (chat-add
backlog lines at 300, `chat ls` preview at 100, prompts, links),
`splitlines()`, `strip()`, `re.sub(r"\s+", ...)` and `ljust` widths count a
lone surrogate as one code point and keep it. Go does the same: `truncRunes`,
`splitlines`, `pyLen` and `pad` step with `decodeWTF8` and slice bytes, never
through `[]rune` (which would turn each surrogate byte into U+FFFD).
`spaceRE` and `pyStrip` use `str.isspace()`'s set (Go's `\s` is ASCII only and
misses `\v` and `\x1c`-`\x1f`). Terminal output is encoded as Python's streams
do it. On stdout (UTF-8 with surrogateescape, the fleet's C.UTF-8 setting)
U+DC80..U+DCFF become the original byte. On stderr (backslashreplace) every
surrogate becomes `\udcXX`. One case is left: Python cannot print other lone
surrogates (such as `\ud800`) to stdout and raises, while Go writes their
WTF-8 bytes. Tests: `TestStringTransformsKeepSurrogates`,
`TestPyStreamEncodesLikePython`, and the interop
`TestInteropLongSurrogateChatAdd`. Python has
`json.loads` semantics as well: `NaN`/`Infinity`, duplicate keys (last value,
first position) and overflow to `inf`. `TestLoadsMatchesPythonJSON` compares
these with CPython when `python3` is installed.

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
5. **`runtime` hook wiring** names this executable's `inbox-hook` directly
   (`hev peers-native inbox-hook`, or the `peers` path that `PEERS_INBOX_BIN`
   names). The hook's ACK instruction names the same entry.
6. **Help** has two extra lines for the folded-in helpers.
7. **Non-UTF-8 tmux output** (a pane whose cwd is not UTF-8) makes the Python
   fail with `UnicodeDecodeError`, because it decodes tmux output strictly. Go
   lists the pane and escapes the bytes as `\udcXX`.
8. **tmux field separator.** Go reads tmux format output intact on every
   tmux and locale tested; the Python does not. Two cases, each needs its own
   fix:
   - **No UTF-8 locale** (`LANG` unset, `LANG=C`, `LC_ALL=C`): tmux prints the
     `0x1f` separator (and any non-ASCII) as `_`. Go runs every tmux command
     with `tmux -u`. The Python happens to get UTF-8 under `LANG=C` or no
     locale (it coerces C to C.UTF-8 and exports `LC_CTYPE`), but not under
     `LC_ALL=C`, where it lists no panes.
   - **tmux 3.4** (Ubuntu 24.04) vis-encodes its output whatever the locale
     or `-u`: control bytes (the separator among them) become `\ooo`, `$`
     before a name becomes `\$`, and a backslash is left as it is. The Python
     splits on the byte and sees no panes: `ls` is empty and every send fails
     with "can't tell which session you are". Replacing `\037` would be
     ambiguous, since a title or cwd can hold those four characters. So when
     the output has no separator byte, Go lists again with every field as
     `#{q:...}`, which doubles each backslash in a field. A `\037` there can
     then only be a separator, and Go decodes `\\$`, `\ooo` and `\X` back to
     the field text (`tmuxFields`, `decodeTmuxQ`). The choice is remembered for
     the rest of the run. 3.6 output is read as before. Tests:
     `TestDecodeTmuxQSplitsTmux34Unambiguously` and
     `TestPanesKeepsLiteralEscapeOnTmux34` (real 3.4 output; title
     `literal\037title`, cwd with `\037`, `$` and `\`). A raw `0x1f` byte
     inside a field still splits it, as on 3.6 and in the Python.

   Checked with two scratch panes, one titled `literal\037title $x`, using
   `ls -a --json`. The table gives agent panes listed (Go / Python), with
   titles exact wherever listed:

   | tmux | no locale | `LANG=C` | `LC_ALL=C` | `LANG=C.UTF-8` |
   |---|---|---|---|---|
   | 3.6 (Oven), without `-u` (one pane, at 181dcb8) | 0 / 1 | 0 / 1 | 0 / 0 | 1 / 1 |
   | 3.6 (Oven), with `-u` (current) | 2 / 2 | 2 / 2 | 2 / 0 | 2 / 2 |
   | 3.4 (Ubuntu 3.4-1build1), current | 2 / 0 | 2 / 0 | 2 / 0 | 2 / 0 |
   | 3.4, `-u` and the old global `\037` replace (181dcb8) | 1 / 0 | 1 / 0 | 1 / 0 | 1 / 0 |

   The 181dcb8 row for 3.4 is from the re-review's reproduction: the
   `normal` pane listed and the `literal\037title` pane was dropped.

   The interop suite needs a tmux that prints the byte (3.6), because on 3.4
   the reference sees nothing to compare. ubuntu-latest ships 3.4, so the
   `peers-interop` CI job builds the pinned tmux 3.6 release from source
   (sha256-checked) and fails unless `tmux -V` is exactly `tmux 3.6`.
9. **ssh deadline.** Every remote call (beacon `_recv`, `ls --everywhere`,
   relays, remote `send`/`inbox`/`chat`) passes `-o ServerAliveInterval=15
   -o ServerAliveCountMax=2` along with `ConnectTimeout=5`, and has a hard
   per-host deadline: 30 s, or `PEERS_SSH_TIMEOUT` seconds. Past the deadline
   ssh is killed, and its pipes are dropped 2 s later even if a child still
   holds them. The call then fails like an unreachable host (exit 255, stderr
   `ssh: timed out after 30s`). The beacon logs `HOST: ...` and goes on to the
   next host. The Python has no deadline: one ssh to Oven hung the laptop's
   beacon for 11.5 h, and launchd starts no new beacon while one is running.
   The native engine's `hev dispatch rpc` ssh gets the same options and a 30 s
   deadline. The beacon's `ssh -G HOST` (its alias lookup, run before any
   transfer) is under the same deadline, since a hanging `Match exec` blocks
   it too. On timeout that host is known by its alias only. Test:
   `TestSSHDeadlineBoundsAStuckHost`, which uses a fake ssh that hangs (also
   on `-G`), places the stuck host before a healthy one, and needs no Python.

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
| `TestInteropLoneSurrogates` | Go beacon + Python sender, and Python beacon + Go sender. A Python-written row with lone surrogate escapes reads back byte-identical (`_chatlog`). Surrogateescape argv from either sender is stored as the same row, and keeps its escapes through outbox, beacon and inbox |
| `TestInteropLongSurrogateChatAdd` | A Go-started chat message of over 300 code points, with surrogateescape bytes on both sides of the 300 cut, `é`, an emoji, `\v` and `\x1c`. Go `chat add` and Python `chat add` write the same backlog line, which keeps `\udcff` and has no U+FFFD. `chat show` and `chat ls` are byte-identical |
| `TestSSHDeadlineBoundsAStuckHost` | Go only (no reference needed; runs in the `go` job). With `PEERS_HOSTS="stuck mini"`, where `stuck` accepts ssh and then hangs, also on `ssh -G`, `beacon` and `ls --everywhere --json` return within the 1 s test deadline. `stuck` is reported as timed out, and `mini` still gets the snapshot or lists its panes |

`cmd/hev/peers_alias_unix_test.go` (`TestPeersHelperAliasesNeverRunBackend`)
builds `hev` and runs it through real `peers`, `peers-inbox-hook` and
`peers-runtime` symlinks with `HEV_PEERS_BACKEND` inherited. It covers send,
the hook's read, its ACK instruction, the runtime's bind and the hook the
runtime wires, and checks that the backend never runs. A control call through
plain `hev peers` shows the backend is still honoured there. It needs tmux but
not the Python.

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

CI's `peers-interop` job is the interop evidence. It checks out hev/skills at
the pinned `947e74f` using the `SKILLS_DEPLOY_KEY` secret, a read-only deploy
key (hev/skills is private). It installs tmux and Python and runs the interop
tests, the alias test and the CPython JSON comparison under `-race`, with
`PEERS_INTEROP_REQUIRED=1`. That variable turns every missing prerequisite into
a failure. The job also fails if any `--- SKIP` line appears, and logs the
number of `--- PASS` lines. The pin matters because the Python files will
leave hev/skills main after the cutover.

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
