---
id: "peers#native-discovery-and-inboxes"
title: "Peers"
heading: "Native discovery and inboxes"
group: "Use"
order: 217
url: "/docs/peers#native-discovery-and-inboxes"
anchor: "native-discovery-and-inboxes"
terms: ["native","discovery","inboxes","peers","built","discovers","local","terminal","sessions","through","tmux","macos","linux","windows","supports","dispatch","reads","interactive","adapter","addresses","name","host","pane","environment","settings","label","remote","hosts","optional","external","backend","session","register","inbox","messages","queued","instead","pasted","peek","read"]
hash: "c6e892b4b538ea232ba9671e1493fc2e1459f2b1b826a5ab2f7a1bb7d80b334f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers inbox on SESSION\nhev peers inbox --peek --pending --json\nhev peers inbox --pending --limit 8 --json\nhev peers inbox ack FULL_ID","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"hev","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"ls","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"read","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"list","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"show","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"NAME","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"HOST/NAME","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"PEERS_SELF","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"PEERS_HOSTS","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"hev peers ls --everywhere","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"HEV_PEERS_BACKEND","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"--peek","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"--pending","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"send --id KEY","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"HOST/ORDER-NAME","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"inbox.md","chunkId":"peers#native-discovery-and-inboxes"},{"kind":"code","literal":"hev dispatch say","chunkId":"peers#native-discovery-and-inboxes"}]
sources: [{"chunkId":"peers#native-discovery-and-inboxes","url":"/docs/peers#native-discovery-and-inboxes","anchor":"native-discovery-and-inboxes"}]
---

Peers is built into hev and discovers local terminal sessions through tmux on macOS and Linux; Windows supports dispatch discovery, log reads and run inboxes but has no interactive terminal adapter. Addresses can be a name, host/name or a pane ID, with environment settings for the host label, remote hosts and an optional external backend. A session can register an inbox so messages are queued instead of pasted, then peek, read a batch and acknowledge each message only after handling it; dispatched runs have stable host/order addresses whose messages go to the run's inbox file.
