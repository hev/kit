---
id: "install#start-the-daemon"
title: "Install"
heading: "Start the daemon"
group: "Start"
order: 191
url: "/docs/install#start-the-daemon"
anchor: "start-the-daemon"
terms: ["start","daemon","bringing","machine","writes","config","file","sets","capture","installs","skills","traces","stay","local","archive","files","nothing","needs","docker","while","exporting","layer","sends","shared","instead","runs","under","launchd","macos","systemd","user","service","linux","enable","lingering","headless","hosts","foreground","supervisor","scheduled"]
hash: "78450c5980e22f4cb5f8efeef823653630c922ce438fd299fadd6cea21c7d876"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev down","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hevd.service","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev daemon install","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"start","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"status","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"stop","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"uninstall","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"loginctl enable-linger","chunkId":"install#start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"install#start-the-daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"install#start-the-daemon"}]
sources: [{"chunkId":"install#start-the-daemon","url":"/docs/install#start-the-daemon","anchor":"start-the-daemon"}]
---

Bringing a machine up writes the config file, sets up the capture daemon and installs skills; with no API key, traces stay in local archive files and nothing needs Docker, while exporting a hev layer key sends the archive to a shared layer instead. The daemon runs under launchd on macOS, a systemd user service on Linux (enable lingering on headless hosts, or run it in the foreground under your own supervisor), and a per-user scheduled task on Windows that needs a separate one-time install step.
