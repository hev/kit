# Native peers cutover (qualification pending)

> **Superseded for the CLI.** `hev peers` now runs the reference-compatible
> port; see [peers-parity.md](peers-parity.md) for its parity table and cutover
> plan. This page remains the record of the #165 native engine, which still
> serves notifications, `--inbox-only`, RPC and the HTTP listener.

Do not perform this cutover yet. The outstanding work is recorded in
[the compatibility audit](peers-native-parity.md).

Build the reviewed kit revision and install its `hev` executable in an isolated
release directory. Record the currently selected `peers` executable and resolve
its symlink target. Preserve that release for rollback. Build and test the same
revision on each supported platform.

Run the old command and `hev peers` against copies of state in separate scratch
HOME directories, with a private tmux socket and fake sessions. Never compare
consuming commands against live state. The reference and native defaults use the
v2 state tree; native tests may set `HEV_PEERS_STATE` to a fixture directory.
Confirm every output/file golden, mixed writers, ACK replay, host relay failures,
plugins and runtime ownership before selecting the native release.

Once qualification is complete, the host operator replaces that host's `peers`
link with a symlink to the reviewed `hev` executable. Its existing basename
handling routes `peers ...` to `hev peers ...`. On platforms without symlink
support, install an equivalent wrapper that forwards all arguments and exit
status. Verify `peers --help`, local message exchange and receipt inspection
through the selected entry point before repeating on another host.

For executable rollback, restore the saved link target. Never reset cursors,
remove receipts, remove a legacy-writer seal or copy an archive over newer state.
The previous executable must understand the active v2 schema; an older transport
requires a separately reviewed stopped-writer migration. Preserve all messages
and receipt journals. Switch plugin/runtime consumers consistently with their
qualified transport; they must not consume from two processes at once.
