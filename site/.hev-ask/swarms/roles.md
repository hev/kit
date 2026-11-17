---
id: "swarms#roles"
title: "Swarms"
heading: "Roles"
group: "Use"
order: 297
url: "/docs/swarms#roles"
anchor: "roles"
terms: ["roles","role","name","shape","order","shapes","partition","unit","piece","race","variant","identical","replicas","single","other","fields","swarm","after","dependencies","stall","duration","effects","branch","draft","choose","mode","races","holds","frontmatter","keys","units","pieces","variants","orchestrator","first","green","person","host","reply","entry"]
hash: "1b6e8a94e7ccab9be3c5e44cf4e04c3756f852678008dfc01ada15330c7cc3cf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"name","chunkId":"swarms#roles"},{"kind":"code","literal":"shape","chunkId":"swarms#roles"},{"kind":"code","literal":"order","chunkId":"swarms#roles"},{"kind":"code","literal":"partition","chunkId":"swarms#roles"},{"kind":"code","literal":"pieces","chunkId":"swarms#roles"},{"kind":"code","literal":"race","chunkId":"swarms#roles"},{"kind":"code","literal":"variants","chunkId":"swarms#roles"},{"kind":"code","literal":"replicas","chunkId":"swarms#roles"},{"kind":"code","literal":"single","chunkId":"swarms#roles"},{"kind":"code","literal":"after","chunkId":"swarms#roles"},{"kind":"code","literal":"stall","chunkId":"swarms#roles"},{"kind":"code","literal":"effects","chunkId":"swarms#roles"},{"kind":"code","literal":"branch","chunkId":"swarms#roles"},{"kind":"code","literal":"pr","chunkId":"swarms#roles"},{"kind":"code","literal":"choose","chunkId":"swarms#roles"},{"kind":"code","literal":"orchestrator","chunkId":"swarms#roles"},{"kind":"code","literal":"first-green","chunkId":"swarms#roles"},{"kind":"code","literal":"person","chunkId":"swarms#roles"},{"kind":"code","literal":"host","chunkId":"swarms#roles"},{"kind":"code","literal":"reply","chunkId":"swarms#roles"}]
sources: [{"chunkId":"swarms#roles","url":"/docs/swarms#roles","anchor":"roles"}]
---

Each role has a name, shape and order. Shapes are partition (one unit per piece), race (one per variant or identical replicas) and single. Other fields set replicas (max 8 per role, 32 per swarm), after dependencies, a stall duration, effects (branch or a single draft PR), and the choose mode for races. The role's order holds order frontmatter keys for its units.
