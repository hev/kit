---
id: "install#start-the-daemon"
title: "Install"
heading: "Start the daemon"
group: "Start"
order: 254
url: "/docs/install#start-the-daemon"
anchor: "start-the-daemon"
terms: ["start","daemon","writes","config","sets","hevd","capture","launchd","macos","systemd","user","service","linux","plus","install","windows","installs","skills","layer","traces","stay","local","files","verify","status","command","toml","archive","hevlayer","down","stop","uninstall","loginctl","enable","linger","starts","machine","needs","write","runs"]
hash: "cd54d105c583e526b6d6959b68faf988712cae46a48e8f6322b439f45f6f8ab8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/traces/","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev down","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd.service","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev daemon install","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"start","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"status","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"stop","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"uninstall","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"loginctl enable-linger","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"install#start-the-daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"install#start-the-daemon"}]
sources: [{"chunkId":"install#start-the-daemon","url":"/docs/install#start-the-daemon","anchor":"start-the-daemon"}]
---

hev up writes config, sets up the hevd capture daemon (launchd on macOS, systemd user service on Linux, plus daemon install on Windows) and installs skills. With no Layer API key, traces stay in local files. Verify with the status command.
