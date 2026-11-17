---
id: "tradeoffs#plain-files-on-each-host"
title: "Tradeoffs"
heading: "Plain files on each host"
group: "Reference"
order: 388
url: "/docs/tradeoffs#plain-files-on-each-host"
anchor: "plain-files-on-each-host"
terms: ["plain","files","host","directory","holding","order","status","notes","report","json","result","readable","removable","ordinary","file","tools","cost","global","view","stored","anywhere","listing","built","asking","every","time","unreachable","drops","names","unique","only","duplicates","require","naming","dispatch","name","rsync","show","stop","under"]
hash: "c7845a9768529e7fb4d2b5b7b84319a19c95e448ec236cbc40ff41ea6b1d1e48"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/dispatch/NAME/","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"cat","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rsync","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"rm","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"hev dispatch ls","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"show","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"stop","chunkId":"tradeoffs#plain-files-on-each-host"},{"kind":"code","literal":"--host","chunkId":"tradeoffs#plain-files-on-each-host"}]
sources: [{"chunkId":"tradeoffs#plain-files-on-each-host","url":"/docs/tradeoffs#plain-files-on-each-host","anchor":"plain-files-on-each-host"}]
---

Each run is a plain directory on the host that ran it, holding the order, status notes, report, log and a JSON result, readable and removable with ordinary file tools. The cost is that no global view is stored anywhere: the run listing is built by asking every host each time, an unreachable host drops out of it, and run names are unique only per host, so duplicates require naming the host.
