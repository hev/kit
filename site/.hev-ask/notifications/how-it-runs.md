---
id: "notifications#how-it-runs"
title: "Notifications"
heading: "How it runs"
group: "Use"
order: 279
url: "/docs/notifications#how-it-runs"
anchor: "how-it-runs"
terms: ["runs","feed","built","hook","handler","only","writes","record","spool","sender","posts","oldest","first","about","second","exits","after","idle","seconds","token","channel","pauses","while","keeping","records","refused","unknown","outcome","messages","dropped","never","doubled","caps","2000","send","starts","none","running","costs","agent"]
hash: "0210ae9f2b779326cfda264a73d3de1cd7805f358cfda9c83b9cb0f41ad9698e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/feed/spool/","chunkId":"notifications#how-it-runs"},{"kind":"code","literal":"hev feed send","chunkId":"notifications#how-it-runs"}]
sources: [{"chunkId":"notifications#how-it-runs","url":"/docs/notifications#how-it-runs","anchor":"how-it-runs"}]
---

The feed is a built-in hook handler that only writes a record to a spool, then a sender posts them oldest first about one a second and exits after 45 idle seconds. Bad token or channel pauses it while keeping records; refused or unknown-outcome messages are dropped, never doubled; spool caps at 2000.
