---
id: "tradeoffs#one-go-binary"
title: "Tradeoffs"
heading: "One Go binary"
group: "Reference"
order: 294
url: "/docs/tradeoffs#one-go-binary"
anchor: "one-go-binary"
terms: ["binary","capture","daemon","search","client","dispatcher","supervisor","local","cross","platform","executable","cost","every","machine","must","compatible","version","since","older","binaries","reject","config","fields","unknown","keys","upgrade","hosts","before","extra","commands","plugins","dispatch","same","source","builds","macos","linux","windows","reports","host"]
hash: "2135d87c8daf7f3899c53d6a931a3589e1810c4dec639592a2fde81e49821b4d"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"tradeoffs#one-go-binary"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"tradeoffs#one-go-binary"}]
sources: [{"chunkId":"tradeoffs#one-go-binary","url":"/docs/tradeoffs#one-go-binary","anchor":"one-go-binary"}]
---

hev is the capture daemon, search client, dispatcher, supervisor and local web UI in one cross-platform executable. The cost is that every machine must run a compatible version, since older binaries reject new config fields as unknown keys, so upgrade hosts before using them. Extra commands go in plugins.
