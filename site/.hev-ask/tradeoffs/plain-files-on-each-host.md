---
id: "tradeoffs#plain-files-on-each-host"
title: "Tradeoffs"
heading: "Plain files on each host"
group: "Reference"
order: 293
url: "/docs/tradeoffs#plain-files-on-each-host"
anchor: "plain-files-on-each-host"
terms: ["plain","files","host","directory","order","status","notes","report","json","result","readable","removable","ordinary","tools","there","stored","global","view","listing","queries","every","live","unreachable","hosts","drop","names","unique","only","duplicates","require","specifying","dispatch","name","rsync","show","stop","under","read","back","delete"]
hash: "c7845a9768529e7fb4d2b5b7b84319a19c95e448ec236cbc40ff41ea6b1d1e48"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/dispatch/NAME/","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"cat","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rsync","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rm","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"hev dispatch ls","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"show","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"stop","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"--host","chunkId":"tradeoffs#plain-files-on-each-host"}]
sources: [{"chunkId":"tradeoffs#plain-files-on-each-host","url":"/docs/tradeoffs#plain-files-on-each-host","anchor":"plain-files-on-each-host"}]
---

Each run is a plain directory on the host that ran it with the order, status notes, report, log and JSON result, readable and removable with ordinary tools. There is no stored global view: listing queries every host live, unreachable hosts drop out, and run names are unique only per host so duplicates require specifying the host.
