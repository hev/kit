---
id: "tradeoffs#the-receiving-host-decides-admission"
title: "Tradeoffs"
heading: "The receiving host decides admission"
group: "Reference"
order: 386
url: "/docs/tradeoffs#the-receiving-host-decides-admission"
anchor: "the-receiving-host-decides-admission"
terms: ["receiving","host","decides","admission","list","repositories","accept","lives","config","checks","clone","real","origin","sender","cannot","widen","bypass","cost","policy","machine","changing","many","hosts","means","editing","central","server","single","place","view","every","repos","accepts","actual","rename","around","point","different","file","change"]
hash: "11eb04a9485e2146651a2206b9c714f0ac5523dc1cfac963a755bfcf5e1b4b87"
mode: "source-primary"
facts: [{"kind":"code","literal":"origin","chunkId":"tradeoffs#the-receiving-host-decides-admission"}]
sources: [{"chunkId":"tradeoffs#the-receiving-host-decides-admission","url":"/docs/tradeoffs#the-receiving-host-decides-admission","anchor":"the-receiving-host-decides-admission"}]
---

The list of repositories a host will accept lives in that host's own config, and the host checks the clone's real origin, so a sender cannot widen or bypass it. The cost is that policy is per machine: changing it on many hosts means editing each one, with no central policy server or single place to view every host's list.
