---
id: "concepts#peers"
title: "Concepts"
heading: "Peers"
group: "Use"
order: 110
url: "/docs/concepts#peers"
anchor: "peers"
terms: ["peers","agent","sessions","live","right","machine","messages","send","other","command","lists","reads","hands","external","backend","path","part","today","fails","without","show","read","message","named","hevpeersbackend"]
hash: "1dc7359fb4597dd5649b11f82743479c0c8e1403df293a846a2bad6ae22c5fb1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers","chunkId":"concepts#peers"},{"kind":"code","literal":"hev peers show","chunkId":"concepts#peers"},{"kind":"code","literal":"hev peers send","chunkId":"concepts#peers"},{"kind":"code","literal":"peers","chunkId":"concepts#peers"},{"kind":"code","literal":"PATH","chunkId":"concepts#peers"},{"kind":"code","literal":"HEV_PEERS_BACKEND","chunkId":"concepts#peers"}]
sources: [{"chunkId":"concepts#peers","url":"/docs/concepts#peers","anchor":"peers"}]
---

Peers are the agent sessions live right now on any machine and the messages they send each other; the peers command lists, reads and messages them. It hands off to an external peers backend on the PATH that is not part of hev kit today, and fails without it.
