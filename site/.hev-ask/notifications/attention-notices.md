---
id: "notifications#attention-notices"
title: "Notifications"
heading: "Attention notices"
group: "Use"
order: 274
url: "/docs/notifications#attention-notices"
anchor: "attention-notices"
terms: ["attention","notices","raised","once","recorded","moment","node","happens","only","open","decision","escalated","halt","needs","runs","hook","handlers","optionally","posts","slack","dispatch","notify","pulled","member","keep","working","table","lists","events","ready","done","name","question","failed","timeout","blocked","review","state","event","hooks"]
hash: "b1a5347ee155121bdd6915607b17bcbe7fc58d1c7e85360d581fa46066c7ddfe"
mode: "agent-primary"
facts: [{"kind":"code","literal":"needs-you","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"[notify.slack]","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"notify","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"needs-decision KEY: QUESTION","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"[needs you] needs-decision KEY: QUESTION","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"ready","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"done","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"failed","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"timeout","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"blocked","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"needs-review","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"state","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"hev.attention.event/v1","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"notifications#attention-notices"},{"kind":"code","literal":"hev.hook.event/v1","chunkId":"notifications#attention-notices"},{"kind":"value","literal":"0.7","chunkId":"notifications#attention-notices"}]
sources: [{"chunkId":"notifications#attention-notices","url":"/docs/notifications#attention-notices","anchor":"attention-notices"}]
---

Attention notices are raised once per recorded moment on the node where it happens; only an open decision or an escalated halt is needs-you. Each runs your hook handlers and optionally posts to Slack; dispatch notify and pulled member notices keep working. Table lists events: needs-you, ready, done.
