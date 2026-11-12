---
id: "install#start-the-daemon"
title: "Install"
heading: "Start the daemon"
group: "Start"
order: 129
url: "/docs/install#start-the-daemon"
anchor: "start-the-daemon"
terms: ["start","daemon","setup","writes","config","installs","capture","skills","without","traces","stay","local","files","nothing","uses","docker","while","exporting","layer","sends","shared","archive","runs","under","launchd","macos","systemd","user","service","linux","enable","lingering","headless","hosts","scheduled","task","windows","installed","separately","toml"]
hash: "fd25a5e4c5c2f5c7c3ca51c82a0646c24e3b801a4c00fd5eb88fb1219c9860b1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev down","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd.service","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev daemon install","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"start","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"status","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"stop","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"uninstall","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"loginctl enable-linger","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"install#start-the-daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"install#start-the-daemon"}]
sources: [{"chunkId":"install#start-the-daemon","url":"/docs/install#start-the-daemon","anchor":"start-the-daemon"}]
---

Setup writes kit's config, installs the capture daemon and skills; without a key traces stay in local files and nothing uses Docker, while exporting a hev layer key sends them to a shared archive. The daemon runs under launchd on macOS, a systemd user service on Linux (enable lingering on headless hosts), and a scheduled task on Windows installed separately.
