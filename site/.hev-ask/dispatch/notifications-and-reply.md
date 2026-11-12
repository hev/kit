---
id: "dispatch#notifications-and-reply"
title: "Dispatch"
heading: "Notifications and reply"
group: "Use"
order: 89
url: "/docs/dispatch#notifications-and-reply"
anchor: "notifications-and-reply"
terms: ["notifications","reply","ends","target","host","runs","configured","notify","command","without","shell","filling","name","state","summary","order","destination","uninterpreted","targets","validation","verdicts","send","second","notification","failures","recorded","changing","dispatch","done","exit","error","result","json","supervisor","reads","config","argument","list","these","placeholders"]
hash: "04621a30964ac864c1c4e3d7e77aab9eacfa33bfb609c33da2b99c72c0d59d66"
mode: "agent-primary"
facts: [{"kind":"code","literal":"notify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"reply","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{name}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{state}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"done","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{summary}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"exit=N","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{reply}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify_error","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"result.json","chunkId":"dispatch#notifications-and-reply"},{"kind":"value","literal":"e.g","chunkId":"dispatch#notifications-and-reply"}]
sources: [{"chunkId":"dispatch#notifications-and-reply","url":"/docs/dispatch#notifications-and-reply","anchor":"notifications-and-reply"}]
---

When a run ends, the target host runs its configured notify command without a shell, filling in name, state, summary and the order's reply destination. Reply is uninterpreted, notify is configured on targets, validation verdicts send a second notification, and notify failures are recorded without changing run state.
