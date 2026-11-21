---
id: "orchestrators#stacking"
title: "Orchestrators"
heading: "Stacking"
group: "Use"
order: 322
url: "/docs/orchestrators#stacking"
anchor: "stacking"
terms: ["stacking","orchestrator","manage","another","only","spec","allows","nesting","child","scope","authority","budget","must","within","parent","chains","stop","cycles","rejected","rarely","good","idea","allow","nested","true","sets","allownested","inside","possible"]
hash: "5f372158762fa8566f91bb62c920fb5e31946b104fefce1d9c7991e829414386"
mode: "agent-primary"
facts: [{"kind":"code","literal":"allow_nested: true","chunkId":"orchestrators#stacking"}]
sources: [{"chunkId":"orchestrators#stacking","url":"/docs/orchestrators#stacking","anchor":"stacking"}]
---

An orchestrator can manage another only if its spec allows nesting; child scope, authority and budget must fit within the parent's, chains stop at two and cycles are rejected. Rarely a good idea.
