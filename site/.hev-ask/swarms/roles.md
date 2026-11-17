---
id: "swarms#roles"
title: "Swarms"
heading: "Roles"
group: "Use"
order: 359
url: "/docs/swarms#roles"
anchor: "roles"
terms: ["roles","role","name","shape","order","partition","makes","unit","piece","race","variant","identical","replicas","single","fields","concurrency","capped","swarm","prerequisite","stall","timeout","whether","pushes","branch","opens","draft","chosen","keys","shared","units","pieces","variants","after","effects","choose","orchestrator","first","green","person","host"]
hash: "1b6e8a94e7ccab9be3c5e44cf4e04c3756f852678008dfc01ada15330c7cc3cf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"name","chunkId":"swarms#roles"},{"kind":"code","literal":"shape","chunkId":"swarms#roles"},{"kind":"code","literal":"order","chunkId":"swarms#roles"},{"kind":"code","literal":"partition","chunkId":"swarms#roles"},{"kind":"code","literal":"pieces","chunkId":"swarms#roles"},{"kind":"code","literal":"race","chunkId":"swarms#roles"},{"kind":"code","literal":"variants","chunkId":"swarms#roles"},{"kind":"code","literal":"replicas","chunkId":"swarms#roles"},{"kind":"code","literal":"single","chunkId":"swarms#roles"},{"kind":"code","literal":"after","chunkId":"swarms#roles"},{"kind":"code","literal":"stall","chunkId":"swarms#roles"},{"kind":"code","literal":"effects","chunkId":"swarms#roles"},{"kind":"code","literal":"branch","chunkId":"swarms#roles"},{"kind":"code","literal":"pr","chunkId":"swarms#roles"},{"kind":"code","literal":"choose","chunkId":"swarms#roles"},{"kind":"code","literal":"orchestrator","chunkId":"swarms#roles"},{"kind":"code","literal":"first-green","chunkId":"swarms#roles"},{"kind":"code","literal":"person","chunkId":"swarms#roles"},{"kind":"code","literal":"host","chunkId":"swarms#roles"},{"kind":"code","literal":"reply","chunkId":"swarms#roles"}]
sources: [{"chunkId":"swarms#roles","url":"/docs/swarms#roles","anchor":"roles"}]
---

Each role has a name, a shape and an order: partition makes one unit per piece, race one per variant (or identical replicas), and single one unit. Role fields set concurrency (capped per role and per swarm), prerequisite roles, a stall timeout, whether the unit pushes a branch or opens the swarm's one draft PR, how a race is chosen, and the order keys shared by its units.
