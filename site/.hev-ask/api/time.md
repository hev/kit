---
id: "api#time"
title: "API overview"
heading: "Time"
group: "API"
order: 19
url: "/docs/api#time"
anchor: "time"
terms: ["time","timestamps","8601","except","trace","session","block","start","prompt","times","stay","unix","epoch","milliseconds","ingest","round","trips","unchanged","suffix","exception","rows","promptts","sessions","blocks","because","stored","attributes","must","trip"]
hash: "8db3b448d1711a448e82d3885efeb1620b65eb62ce2982558df3a15f05ede8ad"
mode: "source-primary"
facts: [{"kind":"code","literal":"start","chunkId":"api#time"},{"kind":"code","literal":"end","chunkId":"api#time"},{"kind":"code","literal":"prompt_ts","chunkId":"api#time"},{"kind":"value","literal":"ISO-8601","chunkId":"api#time"}]
sources: [{"chunkId":"api#time","url":"/docs/api#time","anchor":"time"}]
---

Timestamps are UTC ISO-8601 with Z, except trace session and block start, end and prompt times, which stay Unix epoch milliseconds so ingest round-trips unchanged.
