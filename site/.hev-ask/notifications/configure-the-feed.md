---
id: "notifications#configure-the-feed"
title: "Notifications"
heading: "Configure the feed"
group: "Use"
order: 277
url: "/docs/notifications#configure-the-feed"
anchor: "configure-the-feed"
terms: ["configure","feed","config","table","channel","token","anthropic","secret","references","model","enabled","flag","events","list","default","includes","session","start","stop","needs","ready","done","board","post","evidence","skip","boards","client","roots","upload","size","prefix","name","azkv","status","prompt","submit","tool","compact","peer"]
hash: "389d0d5445110fe2f8e33d8d41087cc60573bcf18e2e54b8422bc0151ab312c4"
mode: "agent-primary"
facts: [{"kind":"code","literal":"token","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"model_key","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"secret://NAME","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"op://...","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"azkv://...","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"hev feed status","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"events","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"session-start","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"prompt-submit","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"pre-tool","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"post-tool","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"stop","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"pre-compact","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"needs-you","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"ready","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"done","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"peer-message","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"board-post","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"evidence","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"boards","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"files:write","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"max_upload_mb","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"HEV_FEED=off","chunkId":"notifications#configure-the-feed"},{"kind":"code","literal":"HEV_HOOKS=off","chunkId":"notifications#configure-the-feed"}]
sources: [{"chunkId":"notifications#configure-the-feed","url":"/docs/notifications#configure-the-feed","anchor":"configure-the-feed"}]
---

Configure the feed in the [feed] config table: channel, bot token and Anthropic key as secret references, model, enabled flag, events list (default includes session-start, stop, needs-you, ready, done, board-post, evidence), skip, boards, client roots, max upload size and a tag prefix.
