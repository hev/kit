---
id: "tradeoffs#plain-files-on-each-host"
title: "Tradeoffs"
heading: "Plain files on each host"
group: "Reference"
order: 349
url: "/docs/tradeoffs#plain-files-on-each-host"
anchor: "plain-files-on-each-host"
terms: ["plain","files","host","directory","easy","read","back","delete","cost","stored","global","view","listing","queries","every","live","offline","hosts","drop","names","unique","only","duplicates","require","specifying","dispatch","name","rsync","show","stop","under","order","status","notes","report","json","result","there","anywhere","builds"]
hash: "c7845a9768529e7fb4d2b5b7b84319a19c95e448ec236cbc40ff41ea6b1d1e48"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/dispatch/NAME/","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"cat","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rsync","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rm","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"hev dispatch ls","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"show","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"stop","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"--host","chunkId":"tradeoffs#plain-files-on-each-host"}]
sources: [{"chunkId":"tradeoffs#plain-files-on-each-host","url":"/docs/tradeoffs#plain-files-on-each-host","anchor":"plain-files-on-each-host"}]
---

Each run is a plain directory on the host that ran it, easy to read, back up or delete. The cost: no stored global view, so listing queries every host live, offline hosts drop out, and run names are unique only per host, so duplicates require specifying the host.
