---
id: "peers#native-discovery-and-inboxes"
title: "Peers"
heading: "Native discovery and inboxes"
group: "Use"
order: 340
url: "/docs/peers#native-discovery-and-inboxes"
anchor: "native-discovery-and-inboxes"
terms: ["native","discovery","inboxes","peers","built","separate","program","read","names","list","show","aliases","local","uses","tmux","macos","linux","while","windows","supports","only","dispatch","reads","addresses","name","host","pane","vars","label","remote","hosts","hold","messages","instead","pasting","terminal","inbox","session","peek","pending"]
hash: "c6e892b4b538ea232ba9671e1493fc2e1459f2b1b826a5ab2f7a1bb7d80b334f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers inbox on SESSION\nhev peers inbox --peek --pending --json\nhev peers inbox --pending --limit 8 --json\nhev peers inbox ack FULL_ID","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"hev","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"ls","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"read","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"list","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"show","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"NAME","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"HOST/NAME","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"PEERS_SELF","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"PEERS_HOSTS","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"hev peers ls --everywhere","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"HEV_PEERS_BACKEND","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"--peek","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"--pending","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"send --id KEY","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"HOST/ORDER-NAME","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"inbox.md","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"hev dispatch say","chunkId":"peers#native-discovery-and-inboxes"}]
sources: [{"chunkId":"peers#native-discovery-and-inboxes","url":"/docs/peers#native-discovery-and-inboxes","anchor":"native-discovery-and-inboxes"}]
---

Peers is built in with no separate program. ls and read are native names (list and show aliases); local discovery uses tmux on macOS/Linux, while Windows supports only dispatch discovery, log reads and run inboxes. Addresses are NAME, HOST/NAME or pane ID; env vars set the host label and remote hosts; inboxes hold messages instead of pasting into a terminal.
