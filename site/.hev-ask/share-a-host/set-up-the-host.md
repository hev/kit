---
id: "share-a-host#set-up-the-host"
title: "Shared hosts"
heading: "Set up the host"
group: "Operate"
order: 339
url: "/docs/share-a-host#set-up-the-host"
anchor: "set-up-the-host"
terms: ["host","before","sharing","complete","headless","setup","install","tmux","authenticated","harness","clone","repos","under","workspace","credentials","keeping","binaries","stable","path","visible","interactive","private","network","such","tailscale","provisions","neither","accounts","needs","public","port","first","agent","keep","seen","noninteractive","absolute","since","members","keys"]
hash: "2271ed683b19e4188fb9b263e8322fb2c75ca42a59d09f56902e2c69cd1db43a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/workspace","chunkId":"share-a-host#set-up-the-host"},{"kind":"code","literal":"hev","chunkId":"share-a-host#set-up-the-host"}]
sources: [{"chunkId":"share-a-host#set-up-the-host","url":"/docs/share-a-host#set-up-the-host","anchor":"set-up-the-host"}]
---

Before sharing, complete headless setup: install hev, Git, tmux and an authenticated harness, clone repos under the workspace, and set up the bot's Git and PR credentials, keeping the binaries at a stable path visible to non-interactive SSH. Put the host on a private network such as Tailscale or a VPN; hev provisions neither network nor OS accounts and needs no public port.
