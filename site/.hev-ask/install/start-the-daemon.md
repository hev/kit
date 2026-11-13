---
id: "install#start-the-daemon"
title: "Install"
heading: "Start the daemon"
group: "Start"
order: 135
url: "/docs/install#start-the-daemon"
anchor: "start-the-daemon"
terms: ["start","daemon","command","writes","config","sets","capture","installs","skills","launchd","macos","systemd","user","service","linux","extra","install","step","windows","layer","traces","stay","local","files","toml","hevd","archive","hevlayer","down","status","stop","uninstall","loginctl","enable","linger","starts","machine","needs","write","runs"]
hash: "9d9e82437782172362eeee085101047f8132c71ba550838393b98f6724fefb80"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev down","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd.service","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev daemon install","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"start","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"status","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"stop","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"uninstall","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"loginctl enable-linger","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"install#start-the-daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"install#start-the-daemon"}]
sources: [{"chunkId":"install#start-the-daemon","url":"/docs/install#start-the-daemon","anchor":"start-the-daemon"}]
---

The up command writes the config, sets up the capture daemon and installs skills, using launchd on macOS, a systemd user service on Linux, and an extra install step on Windows. With no layer key, traces stay in local files.
