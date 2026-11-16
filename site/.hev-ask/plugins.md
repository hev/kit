---
id: "plugins"
title: "Plugins"
heading: null
group: "Operate"
order: 282
url: "/docs/plugins"
anchor: null
terms: ["plugins","subcommands","command","name","built","runs","matching","prefixed","executable","path","passing","arguments","standard","streams","through","returning","exit","code","setup","specific","commands","such","deploy","step","team","report","wrapper","around","internal","tool","args","codetabs","astro","agentprompt","putting","remaining","passed","input","output","error"]
hash: "9788edd99b54ae577d639e3bb07d8d1d93bfd0e7ead576639712ac569c4f81b4"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev NAME args...","chunkId":"plugins"},{"kind":"code","literal":"hev-NAME","chunkId":"plugins"},{"kind":"code","literal":"NAME","chunkId":"plugins"},{"kind":"code","literal":"hev","chunkId":"plugins"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"plugins"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"plugins"}]
sources: [{"chunkId":"plugins","url":"/docs/plugins","anchor":null}]
---

Plugins add your own subcommands: when a command name is not built in, hev runs a matching prefixed executable from your PATH, passing arguments and standard streams through and returning its exit code. Use them for setup-specific commands such as a deploy step, a team report or a wrapper around an internal tool.
