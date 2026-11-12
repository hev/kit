---
id: "windows#capture-traces"
title: "Windows"
heading: "Capture traces"
group: "Reference"
order: 301
url: "/docs/windows#capture-traces"
anchor: "capture-traces"
terms: ["capture","traces","windows","works","alone","layer","runs","scheduled","task","logon","stored","password","startup","triggers","settings","belong","config","file","since","does","later","powershell","variables","daemon","subcommands","show","status","stop","start","uninstall","running","foreground","helps","debugging","reads","codex","claude","code","sessions","user"]
hash: "8080d634447c10b32d82130931d63b0cec5849499fbaf2e969afb52adbae0885"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev daemon status\nhev daemon stop\nhev daemon start\nhev daemon uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"stop","chunkId":"windows#capture-traces"},{"kind":"code","literal":"start","chunkId":"windows#capture-traces"},{"kind":"code","literal":"uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hev daemon","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.codex\\sessions","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.claude\\projects","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon.pid","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon-status.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\index-state.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\hevd.out","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hevd.err","chunkId":"windows#capture-traces"},{"kind":"value","literal":"config.toml","chunkId":"windows#capture-traces"}]
sources: [{"chunkId":"windows#capture-traces","url":"/docs/windows#capture-traces","anchor":"capture-traces"}]
---

Capture on Windows works with kit alone or with layer and runs as a scheduled task using S4U logon (no stored password) with startup and logon triggers; settings belong in the config file since the task does not see later PowerShell variables. Daemon subcommands show status, stop, start or uninstall the task, and running the daemon in the foreground helps debugging. It reads Codex and Claude Code sessions from the user profile and keeps its pid, status, index state and logs under the user's hev folder, readable only by the user, SYSTEM and Administrators.
