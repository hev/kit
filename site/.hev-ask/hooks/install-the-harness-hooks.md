---
id: "hooks#install-the-harness-hooks"
title: "Hooks"
heading: "Install the harness hooks"
group: "Use"
order: 238
url: "/docs/hooks#install-the-harness-hooks"
anchor: "install-the-harness-hooks"
terms: ["install","harness","hooks","dispatch","runs","orchestrator","rounds","wired","automatically","their","supervisor","interactive","sessions","need","check","uninstall","entries","claude","codex","settings","files","every","home","exists","zero","missing","stale","removes","only","json","before","installed","skills","sync","update","nothing","wires","launch","points","code"]
hash: "4ec9e7c4aae0f52781090872afdd2808646ca3375173781390b4137cd4ae4bce"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev hooks install                  # every harness home that exists\nhev hooks install --harness claude\nhev hooks check                    # non-zero when missing or stale\nhev hooks uninstall                # removes only kit's entries","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"~/.claude/settings.json","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"~/.codex/hooks.json","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"$CODEX_HOME","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"settings.json.before-hev-hooks","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"--installed-by kit","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"check","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"uninstall","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"codex","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"/hooks","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"hev skills sync","chunkId":"hooks#install-the-harness-hooks"},{"kind":"code","literal":"hev update","chunkId":"hooks#install-the-harness-hooks"}]
sources: [{"chunkId":"hooks#install-the-harness-hooks","url":"/docs/hooks#install-the-harness-hooks","anchor":"install-the-harness-hooks"}]
---

Dispatch runs and orchestrator rounds get hooks wired automatically by their supervisor; interactive sessions need install, check and uninstall of kit's entries in the Claude and Codex settings files.
