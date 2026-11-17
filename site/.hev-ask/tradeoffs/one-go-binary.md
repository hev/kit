---
id: "tradeoffs#one-go-binary"
title: "Tradeoffs"
heading: "One Go binary"
group: "Reference"
order: 387
url: "/docs/tradeoffs#one-go-binary"
anchor: "one-go-binary"
terms: ["binary","executable","serves","capture","daemon","search","client","dispatcher","supervisor","local","built","same","source","macos","linux","windows","cost","every","machine","must","compatible","version","older","binaries","reject","config","fields","unknown","upgrade","hosts","before","extra","commands","belong","plugins","rather","dispatch","builds","reports","host"]
hash: "2135d87c8daf7f3899c53d6a931a3589e1810c4dec639592a2fde81e49821b4d"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"tradeoffs#one-go-binary"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"tradeoffs#one-go-binary"}]
sources: [{"chunkId":"tradeoffs#one-go-binary","url":"/docs/tradeoffs#one-go-binary","anchor":"one-go-binary"}]
---

One executable serves as capture daemon, search client, dispatcher, supervisor and local web UI, built from the same source for macOS, Linux and Windows. The cost is that every machine must run a compatible version: older binaries reject new config fields as unknown, so upgrade hosts before using them, and extra commands belong in plugins rather than the binary.
