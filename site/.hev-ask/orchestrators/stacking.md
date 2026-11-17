---
id: "orchestrators#stacking"
title: "Orchestrators"
heading: "Stacking"
group: "Use"
order: 205
url: "/docs/orchestrators#stacking"
anchor: "stacking"
terms: ["stacking","orchestrator","manage","another","only","spec","allows","nesting","child","scope","authority","budget","must","inside","parent","chains","stop","levels","cycles","rejected","possible","rarely","good","idea","allow","nested","true","sets","allownested"]
hash: "5f372158762fa8566f91bb62c920fb5e31946b104fefce1d9c7991e829414386"
mode: "agent-primary"
facts: [{"kind":"code","literal":"allow_nested: true","chunkId":"orchestrators#stacking"}]
sources: [{"chunkId":"orchestrators#stacking","url":"/docs/orchestrators#stacking","anchor":"stacking"}]
---

An orchestrator can manage another only if its spec allows nesting. The child's scope, authority and budget must fit inside the parent's, chains stop at two levels and cycles are rejected. It is possible but rarely a good idea.
