---
id: "updating#what-restarts"
title: "Updating and channels"
heading: "What restarts"
group: "Operate"
order: 312
url: "/docs/updating#what-restarts"
anchor: "what-restarts"
terms: ["restarts","after","update","hevd","restarted","service","manager","macos","launchagent","linux","systemd","user","unit","windows","scheduled","task","installed","checked","running","daemon","started","hand","named","report","killed","dispatched","runs","their","supervisors","dashboard","container","launchctl","kickstart","systemctl","restart","platform","checks","keeps","process","stays"]
hash: "bfab5de3035373d5aee7fd126ad2cacb0c933bb309ebafbfe196869d7d05778a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev up","chunkId":"updating#what-restarts"},{"kind":"code","literal":"launchctl kickstart -k","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hevd.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"systemctl --user restart","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev\\hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev d","chunkId":"updating#what-restarts"}]
sources: [{"chunkId":"updating#what-restarts","url":"/docs/updating#what-restarts","anchor":"what-restarts"}]
---

After an update, hevd is restarted by its service manager on macOS (LaunchAgent), Linux (systemd user unit) and Windows (scheduled task), if hev up installed it, and checked to be running. A daemon started by hand is named in the report, not killed. Dispatched runs, their supervisors and the dashboard container are not restarted.
