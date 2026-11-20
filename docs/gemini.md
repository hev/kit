# Gemini CLI

Use `harness: gemini` in dispatch orders, orchestrators and swarms. Install
`@google/gemini-cli` on the execution host and authenticate it there, for
example by supplying `GEMINI_API_KEY` through the launch environment or
`[harnesses.gemini.env_refs]` in dispatch configuration. Kit
uses Gemini's stream-JSON output and requests YOLO approval mode. Gemini's
workspace trust and administrator policies still apply. Omit `model` to use
Gemini's default. Kit's effort setting has no Gemini CLI equivalent here.

Trace indexing reads Gemini CLI's `~/.gemini/tmp/<project>/chats/session-*`
JSON and JSONL files. Current Gemini CLI versions use a registered project
slug and legacy versions use a SHA-256 hash of the project root. Kit reads
both layouts and uses the project marker or registry when available to
recover the workspace path. Antigravity sessions use their own trace store.

`hev hooks install --harness gemini` merges kit hooks into
`~/.gemini/settings.json`. The resulting hook groups can also be placed in
project `.gemini/settings.json` and approved through Gemini's project hook
trust flow. Check and uninstall operate on the user settings file. Kit
does not inject launch-specific Gemini hooks, so dispatch steering uses the
turn boundary.

Kit discovers `GEMINI.md` instructions, Gemini user and workspace skills,
the `.agents/skills` aliases, and user extension skill directories. Peers
shows Gemini CLI processes and their pane state. Gemini peer delivery is
disabled.
