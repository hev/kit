---
id: "windows#capture-traces"
title: "Windows"
heading: "Capture traces"
group: "Reference"
order: 460
url: "/docs/windows#capture-traces"
anchor: "capture-traces"
terms: ["capture","traces","trace","windows","works","every","tier","without","docker","runs","scheduled","task","logon","stored","password","repeating","startup","triggers","configured","config","file","daemon","status","stop","start","uninstall","userprofile","codex","sessions","claude","projects","json","index","state","hevd","toml","upgrade","macos","linux","starter"]
hash: "5ac348ed7f6e1ad8335ebc51133478c0b6302277ba3cd4a3398042f389680df5"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev daemon status\nhev daemon stop\nhev daemon start\nhev daemon uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"stop","chunkId":"windows#capture-traces"},{"kind":"code","literal":"start","chunkId":"windows#capture-traces"},{"kind":"code","literal":"uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hev daemon","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.codex\\sessions","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.claude\\projects","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon.pid","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon-status.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\index-state.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\hevd.out","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hevd.err","chunkId":"windows#capture-traces"},{"kind":"value","literal":"config.toml","chunkId":"windows#capture-traces"}]
sources: [{"chunkId":"windows#capture-traces","url":"/docs/windows#capture-traces","anchor":"capture-traces"}]
---

Trace capture on Windows works on every tier without Docker or a key; it runs as a scheduled task using S4U logon (no stored password) with repeating startup and logon triggers, configured in the config file.
