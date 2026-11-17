---
id: "updating#what-restarts"
title: "Updating and channels"
heading: "What restarts"
group: "Operate"
order: 405
url: "/docs/updating#what-restarts"
anchor: "what-restarts"
terms: ["restarts","update","capture","daemon","installed","secrets","cache","through","platform","service","manager","launch","agents","macos","user","units","linux","scheduled","task","windows","checked","stay","afterwards","started","hand","named","report","killed","dispatched","runs","their","supervisors","restarted","dashboard","container","left","alone","restarting","sends","request"]
hash: "a0382bedaad4661ec0fc222b10f0d80c31962a1d7fd48177b1c4387a1102f7ab"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev up","chunkId":"updating#what-restarts"},{"kind":"code","literal":"launchctl kickstart -k","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.secrets","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hevd.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"systemctl --user restart","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev-secrets.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev\\hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev d","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"updating#what-restarts"}]
sources: [{"chunkId":"updating#what-restarts","url":"/docs/updating#what-restarts","anchor":"what-restarts"}]
---

An update restarts the capture daemon and, where installed, the secrets cache through the platform's service manager: launch agents on macOS, user units on Linux, a scheduled task on Windows, each checked to stay up afterwards. A daemon you started by hand is named in the report but not killed, dispatched runs and their supervisors are not restarted, and the dashboard container is left alone. Restarting the secrets cache sends no request: it refills on demand and any rate-limit cooldown on disk still holds.
