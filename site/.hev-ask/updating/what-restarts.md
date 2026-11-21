---
id: "updating#what-restarts"
title: "Updating and channels"
heading: "What restarts"
group: "Operate"
order: 450
url: "/docs/updating#what-restarts"
anchor: "what-restarts"
terms: ["restarts","table","after","update","platform","hevd","secrets","cache","under","launchagent","macos","systemd","units","linux","scheduled","task","windows","launchctl","kickstart","service","systemctl","user","restart","daemon","only","restarted","installed","checks","keeps","running","process","loaded","unit","stays","active","ends","runs","waits","report","started"]
hash: "a0382bedaad4661ec0fc222b10f0d80c31962a1d7fd48177b1c4387a1102f7ab"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev up","chunkId":"updating#what-restarts"},{"kind":"code","literal":"launchctl kickstart -k","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.secrets","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hevd.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"systemctl --user restart","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev-secrets.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev\\hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev d","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"updating#what-restarts"}]
sources: [{"chunkId":"updating#what-restarts","url":"/docs/updating#what-restarts","anchor":"what-restarts"}]
---

Table of what restarts after an update per platform: hevd and the secrets cache under LaunchAgent on macOS, systemd units on Linux, scheduled task on Windows.
