---
id: "swarms#check-it-first"
title: "Swarms"
heading: "Check it first"
group: "Use"
order: 349
url: "/docs/swarms#check-it-first"
anchor: "check-it-first"
terms: ["check","first","writes","nothing","prints","summary","roles","units","compiled","orchestrator","spec","every","unit","order","scoped","swarm","messaging","rework","retry","dispatch","stop","authority","plus","validate","browser","validation","plain","branch","whose","body","combines","problem","role","section","assignment","rules","acceptance","logfix","name","notify"]
hash: "32ec8d0dc588240f9c14cebe948927a8186ee94908789accd4be118c4cfbc12d"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev swarm up -f logfix.md --dry-run","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"--- orchestrator NAME ---","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"--- order RUN ---","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"swarm=NAME","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"say","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"notify","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"rework","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"retry","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"dispatch","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"stop","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"validate","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"NAME-ROLE-PIECE-g1","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"NAME-ROLE-vN-g1","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"NAME-ROLE-g1","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"d/","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"effects: branch","chunkId":"swarms#check-it-first"},{"kind":"code","literal":"after","chunkId":"swarms#check-it-first"}]
sources: [{"chunkId":"swarms#check-it-first","url":"/docs/swarms#check-it-first","anchor":"check-it-first"}]
---

A dry run writes nothing and prints a summary of roles and units, the compiled orchestrator spec and every unit's order. The compiled orchestrator is scoped to the swarm with messaging, rework, retry, dispatch and stop authority (plus validate when browser validation is used), and each unit is a plain order on its own branch whose body combines the problem, role section, assignment, swarm rules and role acceptance.
