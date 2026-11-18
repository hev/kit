---
id: "orchestrators#stacking"
title: "Orchestrators"
heading: "Stacking"
group: "Use"
order: 259
url: "/docs/orchestrators#stacking"
anchor: "stacking"
terms: ["stacking","orchestrator","manage","another","nesting","explicitly","allowed","child","scope","authority","budget","must","within","parent","chains","limited","levels","cycles","rejected","possible","rarely","good","idea","allow","nested","true","spec","sets","allownested","inside","stop"]
hash: "5f372158762fa8566f91bb62c920fb5e31946b104fefce1d9c7991e829414386"
mode: "agent-primary"
facts: [{"kind":"code","literal":"allow_nested: true","chunkId":"orchestrators#stacking"}]
sources: [{"chunkId":"orchestrators#stacking","url":"/docs/orchestrators#stacking","anchor":"stacking"}]
---

One orchestrator can manage another if nesting is explicitly allowed; the child's scope, authority and budget must fit within the parent's, chains are limited to two levels and cycles are rejected. It is possible but rarely a good idea.
