---
id: "tradeoffs#plain-files-on-each-host"
title: "Tradeoffs"
heading: "Plain files on each host"
group: "Reference"
order: 295
url: "/docs/tradeoffs#plain-files-on-each-host"
anchor: "plain-files-on-each-host"
terms: ["plain","files","host","directory","order","notes","report","json","result","readable","removable","ordinary","tools","cost","stored","global","view","asks","every","live","unreachable","hosts","drop","names","unique","only","show","stop","need","flag","dispatch","name","rsync","under","status","read","back","delete","there","anywhere"]
hash: "c7845a9768529e7fb4d2b5b7b84319a19c95e448ec236cbc40ff41ea6b1d1e48"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/dispatch/NAME/","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"cat","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rsync","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rm","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"hev dispatch ls","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"show","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"stop","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"--host","chunkId":"tradeoffs#plain-files-on-each-host"}]
sources: [{"chunkId":"tradeoffs#plain-files-on-each-host","url":"/docs/tradeoffs#plain-files-on-each-host","anchor":"plain-files-on-each-host"}]
---

Each run is a directory on the host that ran it with the order, notes, report, log and JSON result, readable and removable with ordinary tools. The cost is no stored global view: ls asks every host live, unreachable hosts drop out, and run names are unique only per host so show and stop may need a host flag.
