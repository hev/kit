---
id: "swarms"
title: "Swarms"
heading: null
group: "Use"
order: 252
url: "/docs/swarms"
anchor: null
terms: ["swarm","markdown","file","declaring","problem","orchestrator","worker","roles","concurrency","limits","compiles","ordinary","dispatch","runs","code","only","reconcile","tick","launches","units","repairs","failed","ones","starts","integration","person","merges","phase","every","unit","host","placement","takeover","scale","plan","driven","pieces","come","later","0013"]
hash: "f8c91c71b1f0684fe226ff571d7d340c1d2283556ae803cc7db3df9f647e68ef"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev swarm up","chunkId":"swarms"},{"kind":"code","literal":"up","chunkId":"swarms"},{"kind":"code","literal":"hev swarm scale","chunkId":"swarms"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"swarms"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"swarms"},{"kind":"value","literal":"github.com","chunkId":"swarms"},{"kind":"value","literal":"0013-swarms.md","chunkId":"swarms"}]
sources: [{"chunkId":"swarms","url":"/docs/swarms","anchor":null}]
---

A swarm is one Markdown file declaring a problem, an orchestrator and worker roles with concurrency limits. hev swarm up compiles it into an ordinary orchestrator and dispatch runs, and a code-only reconcile tick launches units, repairs failed ones and starts integration. A person merges. Phase 1 runs every unit on the host where up is run; placement, takeover, scale and plan-driven pieces come later (RFC 0013).
