---
id: "share-a-host#set-up-the-host"
title: "Shared hosts"
heading: "Set up the host"
group: "Operate"
order: 250
url: "/docs/share-a-host#set-up-the-host"
anchor: "set-up-the-host"
terms: ["host","headless","setup","first","install","tmux","authenticated","harness","clone","repos","under","workspace","credentials","keep","stable","absolute","paths","visible","noninteractive","private","network","such","tailscale","public","dashboard","port","needed","does","provision","complete","agent","path","seen","since","members","keys","refer","directly","reach","allow"]
hash: "2271ed683b19e4188fb9b263e8322fb2c75ca42a59d09f56902e2c69cd1db43a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/workspace","chunkId":"share-a-host#set-up-the-host"},{"kind":"code","literal":"hev","chunkId":"share-a-host#set-up-the-host"}]
sources: [{"chunkId":"share-a-host#set-up-the-host","url":"/docs/share-a-host#set-up-the-host","anchor":"set-up-the-host"}]
---

Do headless setup first: install hev, Git, tmux and an authenticated harness, clone repos under the workspace, and set up the bot's Git and PR credentials. Keep hev and the harness at stable absolute paths visible to noninteractive SSH, and put the host on a private network such as Tailscale or a VPN. No public SSH or dashboard port is needed and hev does not provision the network.
