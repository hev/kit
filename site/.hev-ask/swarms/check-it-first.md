---
id: "swarms#check-it-first"
title: "Swarms"
heading: "Check it first"
group: "Use"
order: 287
url: "/docs/swarms#check-it-first"
anchor: "check-it-first"
terms: ["check","first","swarm","writes","nothing","prints","summary","roles","units","compiled","orchestrator","spec","unit","order","refuses","misspelt","fields","later","phases","manages","runs","tagged","name","logfix","notify","rework","retry","dispatch","stop","validate","role","piece","effects","branch","after","file","without","starting","anything","host"]
hash: "32ec8d0dc588240f9c14cebe948927a8186ee94908789accd4be118c4cfbc12d"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev swarm up -f logfix.md --dry-run","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"--- orchestrator NAME ---","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"--- order RUN ---","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"swarm=NAME","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"say","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"notify","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"rework","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"retry","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"dispatch","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"stop","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"validate","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"NAME-ROLE-PIECE-g1","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"NAME-ROLE-vN-g1","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"NAME-ROLE-g1","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"d/","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"effects: branch","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"after","chunkId":"swarms#check-it-first"}]
sources: [{"chunkId":"swarms#check-it-first","url":"/docs/swarms#check-it-first","anchor":"check-it-first"}]
---

A dry run of swarm up writes nothing and prints a summary of roles and units, the compiled orchestrator spec and each unit's order. It refuses misspelt fields and fields from later phases. The compiled orchestrator manages runs tagged with the swarm name.
