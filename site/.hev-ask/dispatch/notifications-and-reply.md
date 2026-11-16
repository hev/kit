---
id: "dispatch#notifications-and-reply"
title: "Dispatch"
heading: "Notifications and reply"
group: "Use"
order: 151
url: "/docs/dispatch#notifications-and-reply"
anchor: "notifications-and-reply"
terms: ["notifications","reply","ends","supervisor","target","host","runs","notify","command","argument","list","without","shell","placeholders","order","name","final","state","summary","field","dispatch","does","interpret","value","destination","understands","configured","validation","verdict","sends","second","notification","failed","recorded","changing","done","exit","error","result","json"]
hash: "04621a30964ac864c1c4e3d7e77aab9eacfa33bfb609c33da2b99c72c0d59d66"
mode: "agent-primary"
facts: [{"kind":"code","literal":"notify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"reply","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{name}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{state}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"done","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{summary}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"exit=N","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{reply}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify_error","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"result.json","chunkId":"dispatch#notifications-and-reply"},{"kind":"value","literal":"e.g","chunkId":"dispatch#notifications-and-reply"}]
sources: [{"chunkId":"dispatch#notifications-and-reply","url":"/docs/dispatch#notifications-and-reply","anchor":"notifications-and-reply"}]
---

When a run ends, the supervisor on the target host runs that host's notify command, an argument list run without a shell, with placeholders for the order name, final state, a summary and the order's reply field. Dispatch does not interpret the reply value; it is a destination your notify command understands. Notify is configured on each target, a validation verdict sends a second notification, and a failed notification is recorded without changing the run's state.
