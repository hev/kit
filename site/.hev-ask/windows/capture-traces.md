---
id: "windows#capture-traces"
title: "Windows"
heading: "Capture traces"
group: "Reference"
order: 414
url: "/docs/windows#capture-traces"
anchor: "capture-traces"
terms: ["capture","traces","trace","windows","works","every","tier","starter","needs","docker","daemon","runs","scheduled","task","stores","password","startup","logon","triggers","repeating","minute","settings","must","config","file","rather","shell","variables","cannot","there","commands","check","stop","start","uninstall","keeps","logs","state","foreground","mode"]
hash: "5ac348ed7f6e1ad8335ebc51133478c0b6302277ba3cd4a3398042f389680df5"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev daemon status\nhev daemon stop\nhev daemon start\nhev daemon uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"stop","chunkId":"windows#capture-traces"},{"kind":"code","literal":"start","chunkId":"windows#capture-traces"},{"kind":"code","literal":"uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hev daemon","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.codex\\sessions","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.claude\\projects","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon.pid","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon-status.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\index-state.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\hevd.out","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hevd.err","chunkId":"windows#capture-traces"},{"kind":"value","literal":"config.toml","chunkId":"windows#capture-traces"}]
sources: [{"chunkId":"windows#capture-traces","url":"/docs/windows#capture-traces","anchor":"capture-traces"}]
---

Trace capture on Windows works on every tier, and Starter Kit needs no Docker or key. The daemon runs as a scheduled task that stores no password and has startup and logon triggers repeating every minute, so settings must go in the config file rather than shell variables the task cannot see. There are commands to check, stop, start and uninstall the task (uninstall keeps config, logs and capture state) and a foreground mode for troubleshooting; capture reads Codex and Claude Code sessions from the user profile, skips missing folders, and keeps its state and output files readable only by you, SYSTEM and Administrators.
