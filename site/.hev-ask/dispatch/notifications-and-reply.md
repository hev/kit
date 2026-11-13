---
id: "dispatch#notifications-and-reply"
title: "Dispatch"
heading: "Notifications and reply"
group: "Use"
order: 95
url: "/docs/dispatch#notifications-and-reply"
anchor: "notifications-and-reply"
terms: ["notifications","reply","ends","supervisor","target","runs","host","notify","command","argument","list","without","shell","placeholders","name","state","summary","order","field","dispatch","done","exit","error","result","json","reads","config","these","replaced","placeholder","value","final","validation","verdict","present","empty","string","notification","toml","takes"]
hash: "04621a30964ac864c1c4e3d7e77aab9eacfa33bfb609c33da2b99c72c0d59d66"
mode: "agent-primary"
facts: [{"kind":"code","literal":"notify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"reply","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{name}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{state}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"done","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{summary}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"exit=N","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{reply}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify_error","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"result.json","chunkId":"dispatch#notifications-and-reply"},{"kind":"value","literal":"e.g","chunkId":"dispatch#notifications-and-reply"}]
sources: [{"chunkId":"dispatch#notifications-and-reply","url":"/docs/dispatch#notifications-and-reply","anchor":"notifications-and-reply"}]
---

When a run ends, the supervisor on the target runs that host's notify command, an argument list without a shell, with placeholders for name, state, summary and the order's reply field.
