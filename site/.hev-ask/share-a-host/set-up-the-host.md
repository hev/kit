---
id: "share-a-host#set-up-the-host"
title: "Shared hosts"
heading: "Set up the host"
group: "Operate"
order: 325
url: "/docs/share-a-host#set-up-the-host"
anchor: "set-up-the-host"
terms: ["host","before","sharing","complete","headless","setup","install","tmux","authenticated","harness","stable","paths","visible","interactive","clone","repos","workspace","configure","credentials","private","network","allowed","does","provision","networks","accounts","needs","public","ports","first","agent","under","keep","path","seen","noninteractive","absolute","since","members","keys"]
hash: "2271ed683b19e4188fb9b263e8322fb2c75ca42a59d09f56902e2c69cd1db43a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/workspace","chunkId":"share-a-host#set-up-the-host"},{"kind":"code","literal":"hev","chunkId":"share-a-host#set-up-the-host"}]
sources: [{"chunkId":"share-a-host#set-up-the-host","url":"/docs/share-a-host#set-up-the-host","anchor":"set-up-the-host"}]
---

Before sharing, complete headless setup: install hev, Git, tmux and an authenticated harness at stable paths visible to non-interactive SSH, clone repos into the workspace, configure the bot's Git and PR credentials, and put the host on a private network with SSH allowed. hev does not provision networks or OS accounts and needs no public ports.
