# Native peers

Start an interactive session with `hev kit` (also available as `hev foh`). It
appears in `hev peers ls` as `kit-<pid>` with its current `idle` or `working`
state. Use that address with the bundled `peers` helper or `hev peers`:

```sh
hev peers ls
hev peers send kit-12345 "The build finished."
hev peers send kit-12345 "Please look at the build result now." --interrupt
```

A local shell send is attributed to `local`. The native harness passes its own
address to child tools. An explicit `--from SESSION` accompanies sends from a
named third-party session.

Local native delivery works on macOS, Linux and Windows without a tmux server.
Each session registers under `~/.hev/peers/native/`. Closing the harness removes
its registration and pending inbox. Discovery ignores old process lifetimes and
dead processes. The private inbox holds up to 128 pending messages and
256 KiB of JSONL, with a 16 KiB limit per message body. Sends report when the
inbox fills.

When a native harness runs inside tmux, the native listing takes the place of
its pane entry. Claude Code, Codex, Antigravity and PI continue to use the
existing tmux discovery and delivery rules, including registered inboxes and
PI's inbox-only delivery. Use `--interrupt` for native recipients.

See [Native front-of-house harness](foh-harness.md) for how the terminal shows
incoming messages.
