---
id: "hooks#notifications"
title: "Hooks"
heading: "Notifications"
group: "Use"
order: 240
url: "/docs/hooks#notifications"
anchor: "notifications"
terms: ["notifications","notification","handler","attached","hook","event","kinds","documented","page","attention","notices","needs","ready","done","deduplicated","through","handlers","optionally","slack","pull","commands","emitting","ones","held","hosts","cannot","post","feed","narrates","sessions","runs","board","posts","evidence","channel","name","notify","dispatch","orchestrate","their"]
hash: "a80addcb46bb9405766fa4fc9125731f9811575e9de04242db5ad528a2ec00f8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"needs-you","chunkId":"hooks#notifications"},{"kind":"code","literal":"ready","chunkId":"hooks#notifications"},{"kind":"code","literal":"done","chunkId":"hooks#notifications"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"hooks#notifications"},{"kind":"code","literal":"[notify.slack]","chunkId":"hooks#notifications"},{"kind":"code","literal":"hev dispatch notices pull","chunkId":"hooks#notifications"},{"kind":"code","literal":"hev orchestrate notices pull","chunkId":"hooks#notifications"},{"kind":"code","literal":"feed","chunkId":"hooks#notifications"},{"kind":"code","literal":"[feed]","chunkId":"hooks#notifications"},{"kind":"code","literal":"hev feed","chunkId":"hooks#notifications"}]
sources: [{"chunkId":"hooks#notifications","url":"/docs/hooks#notifications","anchor":"notifications"}]
---

A notification is a handler attached to a hook event; kit has two kinds, now documented on the Notifications page: attention notices (needs-you, ready, done, deduplicated, run through hook handlers and optionally Slack, with notices pull commands re-emitting ones held on hosts that cannot post) and the Slack feed handler that narrates sessions, runs, board posts and evidence into one channel.
