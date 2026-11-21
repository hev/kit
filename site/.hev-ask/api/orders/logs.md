---
id: "api/orders#logs"
title: "Orders API"
heading: "Logs"
group: "API"
order: 48
url: "/docs/api/orders#logs"
anchor: "logs"
terms: ["logs","returns","harness","jsonl","output","next","cursor","active","flag","poll","until","longer","curl","https","hevmind","orders","docs","typo","20261120","authorization","bearer","type","assistant","mta0odu3ng","true","exec","json","stream","limit","round","every","hevapikey","nextcursor","codex","claude","false","caps","bytes","returned","default"]
hash: "b0d6f1e07e7f0fe1b78b15437a37a8431c666c238cd5f17a7f5bcf5840dca883"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"https://kit.hevmind.com/v1/orders/kit-docs-typo-20261120/logs\" \\\n  -H \"Authorization: Bearer $HEV_API_KEY\"","chunkId":"api/orders#logs"},{"kind":"code","literal":"{ \"log\": \"{\\\"type\\\":\\\"assistant\\\",…}\\n\", \"next_cursor\": \"MTA0ODU3Ng\", \"active\": true }","chunkId":"api/orders#logs"},{"kind":"code","literal":"log","chunkId":"api/orders#logs"},{"kind":"code","literal":"exec --json","chunkId":"api/orders#logs"},{"kind":"code","literal":"stream-json","chunkId":"api/orders#logs"},{"kind":"code","literal":"next_cursor","chunkId":"api/orders#logs"},{"kind":"code","literal":"cursor","chunkId":"api/orders#logs"},{"kind":"code","literal":"active","chunkId":"api/orders#logs"},{"kind":"code","literal":"limit","chunkId":"api/orders#logs"},{"kind":"code","literal":"round","chunkId":"api/orders#logs"},{"kind":"code","literal":"every","chunkId":"api/orders#logs"}]
sources: [{"chunkId":"api/orders#logs","url":"/docs/api/orders#logs","anchor":"logs"}]
---

Returns the harness's JSONL output with a next cursor and an active flag; poll with the cursor until the run is no longer active.
