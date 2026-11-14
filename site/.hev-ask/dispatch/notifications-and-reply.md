---
id: "dispatch#notifications-and-reply"
title: "Dispatch"
heading: "Notifications and reply"
group: "Use"
order: 144
url: "/docs/dispatch#notifications-and-reply"
anchor: "notifications-and-reply"
terms: ["notifications","reply","ends","target","host","runs","configured","notification","command","without","shell","filling","order","name","final","state","summary","destination","dispatch","passes","through","uninterpreted","validation","verdicts","send","second","notice","failures","recorded","changing","notify","done","exit","error","result","json","supervisor","reads","config","argument"]
hash: "04621a30964ac864c1c4e3d7e77aab9eacfa33bfb609c33da2b99c72c0d59d66"
mode: "agent-primary"
facts: [{"kind":"code","literal":"notify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"reply","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{name}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{state}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"done","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{summary}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"exit=N","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"{reply}","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"notify_error","chunkId":"dispatch#notifications-and-reply"},{"kind":"code","literal":"result.json","chunkId":"dispatch#notifications-and-reply"},{"kind":"value","literal":"e.g","chunkId":"dispatch#notifications-and-reply"}]
sources: [{"chunkId":"dispatch#notifications-and-reply","url":"/docs/dispatch#notifications-and-reply","anchor":"notifications-and-reply"}]
---

When a run ends, the target host runs its configured notification command without a shell, filling in the order name, final state, a summary and the order's reply destination, which dispatch passes through uninterpreted. Notification is configured per target host; validation verdicts send a second notice, and failures are recorded without changing state.
