---
id: "share-a-host#set-up-the-host"
title: "Shared hosts"
heading: "Set up the host"
group: "Operate"
order: 269
url: "/docs/share-a-host#set-up-the-host"
anchor: "set-up-the-host"
terms: ["host","first","complete","headless","setup","tmux","authenticated","agent","harness","repos","cloned","under","workspace","credentials","stable","paths","interactive","path","private","network","such","tailscale","reachable","members","does","provision","networking","accounts","needs","public","ports","install","clone","keep","seen","noninteractive","absolute","since","keys","refer"]
hash: "2271ed683b19e4188fb9b263e8322fb2c75ca42a59d09f56902e2c69cd1db43a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/workspace","chunkId":"share-a-host#set-up-the-host"},{"kind":"code","literal":"hev","chunkId":"share-a-host#set-up-the-host"}]
sources: [{"chunkId":"share-a-host#set-up-the-host","url":"/docs/share-a-host#set-up-the-host","anchor":"set-up-the-host"}]
---

First complete headless setup: hev, Git, tmux and an authenticated agent harness, repos cloned under the workspace, and bot Git and PR credentials, with hev and the harness at stable paths on the non-interactive SSH PATH. Put the host on a private network such as Tailscale or a VPN reachable by members; hev does not provision networking or OS accounts and needs no public ports.
