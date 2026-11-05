# Native peers compatibility audit

This is an incomplete migration. Keep the existing command selected until the
remaining contracts and acceptance checks pass. The Go chat implementation is a
separate protocol and must not be presented as compatible with existing chats.

Reference: Python source revision `a2b71685d2cbc7bd52a8b22787de1018db277cb8`.
The reference stays outside this public repository. No live state was migrated.

| Command / contract | Native status |
| --- | --- |
| default, `ls [-a] [--everywhere] [--json]` | Partial: native discovery exists; cached snapshots, complete JSON metadata, standalone headless discovery and display parity remain |
| `read TARGET [-n N]` | Partial: tmux and dispatch reads exist; transcript rendering and cached screen reads remain; default line count differs |
| `send TARGET MESSAGE\|- [--from SESSION] [--force]` | Partial: native delivery exists; exact provenance header, sender metadata, self-send checks and output parity remain |
| `send --id ID --order ID --run ID` | Provenance and stable identifier validation added; retry conflicts include provenance. HTTP and inbox-only notification callers retain existing opaque IDs |
| bare-name send resolution | Added existing `hosts/*.json` lookup, local preference, offline exclusion and ambiguity rejection; transport still uses existing configured native routing |
| `wait TARGET [--timeout S]` | Missing |
| `role [SESSION] [foh\|boh\|offline]` | Missing; discovery and delivery must enforce roles too |
| `inbox on\|off [SESSION]` | Partial: native toggle exists; exact text differs |
| `inbox [--session S] --peek\|--count\|--json\|--all\|--current-all\|--pending [--limit N]` | Core JSON shape and replay implemented; explicit limit now 1..64. Text, missing-session nulls and duplicate-row filtering remain |
| `inbox hold [--sender X] -` | Core hold, dedupe, incarnation and fsync exist; sender/whitespace parity needs completion |
| `inbox consume ID...`, `ack ID...`, `owner` | Implemented; full IDs, locked validation, separate receipts and replay. Invalid timestamp receipts now ignored like Python |
| `inbox bind-runtime PID`, `PEERS_INBOX_TOKEN` | Binding/delegation missing. Presence of unsupported token now fails closed instead of falling back to pane ownership |
| `managed register NAME --root ABS [--replace]` | Missing: registration, process ancestry, lifetime validation and discovery |
| `beacon`, `_recv` | Missing: snapshots, sent-log byte cursors, transfer receipts, durable fetch and pending delivery |
| unreachable host delivery | Missing: incarnation-bound queue, origin-scoped retry fingerprint, outbox/pending journals and TTL/drop handling |
| `chat new MEMBER... [--title T] [-m MESSAGE\|-]` | Existing Go token protocol differs from Python thread files; replacement required |
| `chat say ID MESSAGE\|-`, `add ID MEMBER... [-m MESSAGE\|-]` | Native say incompatible; add missing |
| `chat leave ID\|--all [--reason TEXT] [--hook HARNESS]` | Missing, including clear hooks and bound membership |
| `chat ls`, `show ID [-n N]`, `_chat`, `_chatlog` | Native ls/show incompatible; remote wire commands missing |
| `help`, `-h`, `--help` | Native flag help exists; command help and full usage parity remain |
| `chef-notify` | Deliberately retired; native should provide the explicit retirement diagnostic |
| `PEERS_TRANSPORT=sql`, `lib/durable.py` | Deliberately dropped: inactive SQL transport; do not port |
| `peers` compatibility entry point | Existing executable-basename dispatch supports a symlink to `hev`; installer/runbook qualification remains |
| Claude mod and PI extension | Not shipped from kit yet; private references must be sanitized and tested before bundling |
| `peers-runtime`, `peers-inbox-hook` | Missing native runtime integration and installation |
| `peers-cutover`, `peers-history` | Migration/reconstruction utilities not ported; retain separately until reviewed replacements exist |

All listed flags are scoped to their command; `--from` is sender attribution and
must never delegate receipt ownership. Runtime launch flags such as `--print`
are classifier inputs, not peers CLI options. `--show-current` belongs to the
Git invocation used to read the branch.

## Storage compatibility

The current Python default and native default are both
`~/.local/state/peers-v2`. The old `~/.local/state/peers` path may be a cutover
seal, not a directory. Do not reinterpret or remove it.

| State path under the v2 tree | Native status |
| --- | --- |
| `inbox/SESSION.jsonl`, `.jsonl.lock`, `.jsonl.receipts.jsonl` | Shared append-only formats; cross-reading tested with supplied Python reference |
| `inbox/SESSION.cursor` | Retained unchanged; not used to adopt unbound history |
| `sent.jsonl` | Missing delivery log writes |
| `hosts/HOST.json`, `hosts/HOST.sent.jsonl` | Alias lookup reads snapshots; snapshot shipping and log ingestion missing |
| `outbox/*.json`, `pending/*.json` | Missing |
| `queued.jsonl`, `queued.jsonl.lock` | Missing durable queue dedupe |
| `relay-transfers.jsonl`, `.lock`, `relayed.jsonl` | Missing transfer completion and relay outcomes |
| `sent-cursor-HOST` | Missing byte-offset sent-log shipping |
| `threads/ID.json` | Missing Python thread schema and incarnation-bound membership |
| `managed/NAME.json` | Missing managed lifetime registry |
| `runtime/TOKEN.json` | Missing launcher delegation capability |
| `cutover-review.json`, `cutover-ready.json` | No native readiness gate; do not deploy as a cutover replacement |
| native `inbox/SESSION.chats.jsonl` | Native-only protocol; not a substitute for Python thread storage |

The retained legacy archive and regular-file seal are owned by the reviewed
migration procedure, not normal messaging. Hook digest/UI state must retain its
original incarnation binding when the plugin is ported.

## Validation

`PEERS_PYTHON_REFERENCE=/path/to/reference/bin/peers go test -race -count=1 ./internal/peers`
checks shared inbox rows and receipts without vendoring private source. The
reference test imports functions in scratch HOME and state, bypassing only its
CLI cutover readiness check; it is not a live end-to-end CLI qualification.

The scratch CLI check also passed on Linux with two fake sessions on a private
tmux socket: native and symlink sends, Python/Go cross-consumption, replay and
explicit ACK. Reproduce with
`python3 scripts/test-peers-interop.py /path/to/hev /path/to/reference/bin/peers`.

Still required: every-command golden outputs and files, concurrent mixed writers,
unreachable relay, complete bare-name delivery, native Windows acceptance,
hook/plugin tests and green platform CI.
The Python reference test skips without the explicit path; a default suite pass
must not be reported as reference parity evidence.

## Storage and restricted SSH routing

The `Store` journal interface supplies locked read/validate/append transactions
for inbox rows and receipts and native chat history. `FileStore` is its only
implementation; existing bytes, paths and fsync behavior are retained. Sent-log
and relay journal work must use the same interface when implemented. Snapshot
files are separate from append-only journals. No cloud backend is included.

SSH executes the fixed `hev dispatch rpc` command. Unrestricted requests carry
`action: peers` and a JSON `note` containing `args` and `stdin`; responses use
`log`, `message`, and `error`. A `send d-NAME` request instead uses the existing
`peer-send` action with `name: NAME` and a provenance-prefixed message in `note`.
The receiving access policy must authenticate the key and check ownership of
that run before permitting `peer-send`. Sender text is attribution, not authority.

The parallel node-access change must allow own-run `peer-send` after its normal
ownership check. It must keep generic `peers` actions denied to restricted keys.
When resolving the RPC handler integration, dispatch generic peers requests only
in the unrestricted branch, after authentication selected that branch. Restricted
keys cannot list/read arbitrary sessions or consume/ACK another recipient's inbox.
Cross-host interactive peers require an unrestricted credential until separately
scoped policy is designed. Stable IDs do not add exactly-once dispatch submission.
