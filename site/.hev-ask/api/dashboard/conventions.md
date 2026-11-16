---
id: "api/dashboard#conventions"
title: "Dashboard API"
heading: "Conventions"
group: "API"
order: 37
url: "/docs/api/dashboard#conventions"
anchor: "conventions"
terms: ["conventions","dashboard","most","timestamps","unix","milliseconds","durations","costs","dollars","every","json","response","errors","included","carries","timing","object","describing","archive","reads","zeros","file","mirrored","header","layer","queries","rows","start","prompt","cost","spend","server","named","promptts","unless","field","says","3339","string","ending"]
hash: "b6920ca4b652ea9c523cedad67d54ad9b9e6e32a210007fb9f34898881a04322"
mode: "source-primary"
facts: [{"kind":"code","literal":"\"timing\": { \"layer_ms\": 0, \"queries\": 0, \"rows\": 0 }","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"start","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"end","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"ts","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"prompt_ts","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"_ms","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"cost","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"spend","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"timing","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"Server-Timing","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"layer;dur=0.000","chunkId":"api/dashboard#conventions"}]
sources: [{"chunkId":"api/dashboard#conventions","url":"/docs/api/dashboard#conventions","anchor":"conventions"}]
---

Dashboard API conventions: most timestamps are Unix milliseconds, durations are milliseconds, costs are US dollars. Every JSON response, errors included, carries a timing object describing archive reads, which is all zeros with the file archive and is mirrored in a response header.
