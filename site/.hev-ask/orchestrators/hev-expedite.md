---
id: "orchestrators#hev-expedite"
title: "Orchestrators"
heading: "hev expedite"
group: "Use"
order: 201
url: "/docs/orchestrators#hev-expedite"
anchor: "hev-expedite"
terms: ["expedite","follows","runs","current","peer","session","dispatched","review","rework","authority","chosen","host","interval","maximum","lifetime","status","stop","subcommands","orchestrator","machinery","follow","through","preset","whereas","orchestrate","start","mine","selector","gives","general","default","notify","buildbox","every","replacing","dispatch","config","work","confirm","running"]
hash: "b4e10985f8733a4251ef7185b38b99c84cf6739f32c4c3d2a14bdd0f75f91e12"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev expedite --host buildbox --authority review --every 10m --max-lifetime 24h\nhev expedite status\nhev expedite stop","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"hev expedite","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"hev orchestrate start --runs mine","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"say","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"notify","chunkId":"orchestrators#hev-expedite"}]
sources: [{"chunkId":"orchestrators#hev-expedite","url":"/docs/orchestrators#hev-expedite","anchor":"hev-expedite"}]
---

hev expedite follows the runs the current peer session dispatched, with review and rework authority, on a chosen host, interval and maximum lifetime. It has status and stop subcommands. It runs on the orchestrator machinery with a follow-through preset, whereas orchestrate start with the mine selector gives a general orchestrator with default say and notify authority.
