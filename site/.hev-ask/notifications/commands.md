---
id: "notifications#commands"
title: "Notifications"
heading: "Commands"
group: "Use"
order: 276
url: "/docs/notifications#commands"
anchor: "commands"
terms: ["commands","feed","status","shows","config","spool","pause","state","sender","evidence","posts","recording","directory","made","elsewhere","replay","runs","recorded","events","through","prints","error","without","putting","secrets","recent","name","post","file","print","hooks","jsonl","check","slack","host","tell","whether","boards","paused","last"]
hash: "733c3f112b70e7bb85f28f5fbcb7d998394f320b172ef9833d252fbd905ea175"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev feed status                       # config, spool, pause and recent sender log\nhev feed evidence DIR [--run NAME]    # post a recording directory made elsewhere\nhev feed replay FILE --print          # run recorded events through the feed and print the posts","chunkId":"notifications#commands"},{"kind":"code","literal":"replay","chunkId":"notifications#commands"},{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"notifications#commands"}]
sources: [{"chunkId":"notifications#commands","url":"/docs/notifications#commands","anchor":"commands"}]
---

Feed commands: status shows config, spool, pause state and sender log; evidence posts a recording directory made elsewhere; replay runs recorded events through the feed and prints the posts. If it's off from a config error, fix the config without putting secrets in it.
