---
id: "install#start-the-daemon"
title: "Install"
heading: "Start the daemon"
group: "Start"
order: 184
url: "/docs/install#start-the-daemon"
anchor: "start-the-daemon"
terms: ["start","daemon","setup","command","writes","config","sets","capture","installs","skills","traces","stay","local","files","docker","while","exporting","layer","sends","shared","archive","runs","under","launchd","macos","systemd","user","service","linux","enable","lingering","headless","hosts","windows","must","installed","separately","scheduled","task","toml"]
hash: "78450c5980e22f4cb5f8efeef823653630c922ce438fd299fadd6cea21c7d876"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev down","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd.service","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev daemon install","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"start","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"status","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"stop","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"uninstall","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"loginctl enable-linger","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"install#start-the-daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"install#start-the-daemon"}]
sources: [{"chunkId":"install#start-the-daemon","url":"/docs/install#start-the-daemon","anchor":"start-the-daemon"}]
---

The setup command writes kit's config, sets up the capture daemon and installs skills; with no key traces stay in local files with no Docker, while exporting a hev layer key sends them to a shared archive. The daemon runs under launchd on macOS and a systemd user service on Linux (enable lingering on headless hosts); on Windows it must be installed separately as a scheduled task.
