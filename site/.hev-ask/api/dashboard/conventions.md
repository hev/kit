---
id: "api/dashboard#conventions"
title: "Dashboard API"
heading: "Conventions"
group: "API"
order: 37
url: "/docs/api/dashboard#conventions"
anchor: "conventions"
terms: ["conventions","timestamps","unix","milliseconds","unless","noted","3339","fields","ending","costs","dollars","every","response","carries","timing","object","about","archive","reads","zeros","file","also","sent","server","header","layer","queries","rows","start","prompt","cost","spend","named","promptts","field","says","string","durations","json","returns"]
hash: "b6920ca4b652ea9c523cedad67d54ad9b9e6e32a210007fb9f34898881a04322"
mode: "source-primary"
facts: [{"kind":"code","literal":"\"timing\": { \"layer_ms\": 0, \"queries\": 0, \"rows\": 0 }","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"start","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"end","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"ts","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"prompt_ts","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"_ms","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"cost","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"spend","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"timing","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"Server-Timing","chunkId":"api/dashboard#conventions"},{"kind":"code","literal":"layer;dur=0.000","chunkId":"api/dashboard#conventions"}]
sources: [{"chunkId":"api/dashboard#conventions","url":"/docs/api/dashboard#conventions","anchor":"conventions"}]
---

Timestamps are Unix milliseconds unless noted as RFC 3339, fields ending in ms are milliseconds, and costs are US dollars. Every response carries a timing object about archive reads (all zeros with the file archive), also sent as a Server-Timing header.
