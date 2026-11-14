---
id: "windows#capture-traces"
title: "Windows"
heading: "Capture traces"
group: "Reference"
order: 363
url: "/docs/windows#capture-traces"
anchor: "capture-traces"
terms: ["capture","traces","trace","windows","works","every","tier","runs","scheduled","task","logon","stored","password","triggered","startup","repeating","minute","settings","belong","config","file","since","later","shell","variables","covers","checking","stopping","starting","uninstalling","foreground","mode","debugging","session","folders","read","private","daemon","state","files"]
hash: "5ac348ed7f6e1ad8335ebc51133478c0b6302277ba3cd4a3398042f389680df5"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev daemon status\nhev daemon stop\nhev daemon start\nhev daemon uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"stop","chunkId":"windows#capture-traces"},{"kind":"code","literal":"start","chunkId":"windows#capture-traces"},{"kind":"code","literal":"uninstall","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hev daemon","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.codex\\sessions","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.claude\\projects","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon.pid","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\daemon-status.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\index-state.json","chunkId":"windows#capture-traces"},{"kind":"code","literal":"%USERPROFILE%\\.hev\\hevd.out","chunkId":"windows#capture-traces"},{"kind":"code","literal":"hevd.err","chunkId":"windows#capture-traces"},{"kind":"value","literal":"config.toml","chunkId":"windows#capture-traces"}]
sources: [{"chunkId":"windows#capture-traces","url":"/docs/windows#capture-traces","anchor":"capture-traces"}]
---

Trace capture on Windows works on every tier and runs as a scheduled task with S4U logon (no stored password), triggered at startup and logon and repeating every minute; settings belong in the config file since the task won't see later shell variables. Covers checking, stopping, starting and uninstalling the task, foreground mode for debugging, which session folders are read, and the private daemon state files.
