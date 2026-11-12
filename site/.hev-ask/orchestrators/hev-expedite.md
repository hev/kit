---
id: "orchestrators#hev-expedite"
title: "Orchestrators"
heading: "hev expedite"
group: "Use"
order: 195
url: "/docs/orchestrators#hev-expedite"
anchor: "hev-expedite"
terms: ["expedite","follows","runs","current","peer","session","dispatched","review","rework","authority","cadence","lifetime","built","orchestrator","machinery","general","defaults","message","notify","host","buildbox","every","status","stop","orchestrate","start","mine","follow","replacing","dispatch","config","work","confirm","running","only","docs","https","hevkit","orchestrators","pages"]
hash: "b4e10985f8733a4251ef7185b38b99c84cf6739f32c4c3d2a14bdd0f75f91e12"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev expedite --host buildbox --authority review --every 10m --max-lifetime 24h\nhev expedite status\nhev expedite stop","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"hev expedite","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"hev orchestrate start --runs mine","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"say","chunkId":"orchestrators#hev-expedite"},{"kind":"code","literal":"notify","chunkId":"orchestrators#hev-expedite"}]
sources: [{"chunkId":"orchestrators#hev-expedite","url":"/docs/orchestrators#hev-expedite","anchor":"hev-expedite"}]
---

Expedite follows the runs your current peer session dispatched with review and rework authority on a cadence and lifetime, built on the orchestrator machinery; a general orchestrator over your own runs defaults to message and notify authority.
