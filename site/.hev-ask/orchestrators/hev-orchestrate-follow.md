---
id: "orchestrators#hev-orchestrate-follow"
title: "Orchestrators"
heading: "hev orchestrate follow"
group: "Use"
order: 201
url: "/docs/orchestrators#hev-orchestrate-follow"
anchor: "hev-orchestrate-follow"
terms: ["orchestrate","follow","follows","runs","current","peer","session","dispatched","review","rework","authority","chosen","host","interval","maximum","lifetime","status","stop","subcommands","orchestrator","follow-through","preset","whereas","start","mine","selector","gives","general","default","notify","buildbox","every","replacing","dispatch","config","work","confirm","running","expedite","expediter","deprecated","alias"]
hash: "b4e10985f8733a4251ef7185b38b99c84cf6739f32c4c3d2a14bdd0f75f91e12"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate follow --host buildbox --authority review --every 10m --max-lifetime 24h\nhev orchestrate follow status\nhev orchestrate follow stop","chunkId":"orchestrators#hev-orchestrate-follow"},{"kind":"code","literal":"hev orchestrate follow","chunkId":"orchestrators#hev-orchestrate-follow"},{"kind":"code","literal":"hev orchestrate start --runs mine","chunkId":"orchestrators#hev-orchestrate-follow"},{"kind":"code","literal":"say","chunkId":"orchestrators#hev-orchestrate-follow"},{"kind":"code","literal":"notify","chunkId":"orchestrators#hev-orchestrate-follow"},{"kind":"code","literal":"hev expedite","chunkId":"orchestrators#hev-orchestrate-follow"}]
sources: [{"chunkId":"orchestrators#hev-orchestrate-follow","url":"/docs/orchestrators#hev-orchestrate-follow","anchor":"hev-orchestrate-follow"}]
---

hev orchestrate follow starts a follow-through orchestrator for the runs the current peer session dispatched, with review and rework authority, on a chosen host, interval and maximum lifetime. It has status and stop subcommands. It is an ordinary orchestrator with a follow-through preset, whereas orchestrate start with the mine selector gives a general orchestrator with default say and notify authority. Earlier releases called it an Expediter and the command hev expedite, which still works as a deprecated alias.
