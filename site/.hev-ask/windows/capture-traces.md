---
id: "windows#capture-traces"
title: "Windows"
heading: "Capture traces"
group: "Reference"
order: 346
url: "/docs/windows#capture-traces"
anchor: "capture-traces"
terms: ["capture","traces","works","standalone","layer","docker","needed","scheduled","task","uses","logon","password","stored","startup","triggers","repeating","every","minute","settings","config","file","shell","daemon","status","stop","start","uninstall","commands","checking","before","changing","anything","userprofile","codex","sessions","claude","projects","json","index","state"]
hash: "8080d634447c10b32d82130931d63b0cec5849499fbaf2e969afb52adbae0885"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev daemon status\nhev daemon stop\nhev daemon start\nhev daemon uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"stop","chunkId":"windows#capture-traces"},{"kind":"code","literal":"start","chunkId":"windows#capture-traces"},{"kind":"code","literal":"uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hev daemon","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.codex\\sessions","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.claude\\projects","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon.pid","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon-status.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\index-state.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\hevd.out","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hevd.err","chunkId":"windows#capture-traces"},{"kind":"value","literal":"config.toml","chunkId":"windows#capture-traces"}]
sources: [{"chunkId":"windows#capture-traces","url":"/docs/windows#capture-traces","anchor":"capture-traces"}]
---

Capture works standalone or with a layer key, with no Docker needed standalone. The scheduled task uses S4U logon so no password is stored and has startup and logon triggers repeating every minute. Put settings in the config file, not the shell, and use the daemon status, stop, start and uninstall commands, checking status before changing anything.
