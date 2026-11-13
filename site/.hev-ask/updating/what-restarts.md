---
id: "updating#what-restarts"
title: "Updating and channels"
heading: "What restarts"
group: "Operate"
order: 343
url: "/docs/updating#what-restarts"
anchor: "what-restarts"
terms: ["restarts","update","hevd","installed","running","standalone","secrets","cache","service","macos","launchd","linux","systemd","user","units","scheduled","task","windows","checking","comes","back","hand","started","daemons","dispatched","runs","dashboard","container","restarted","restarting","sends","request","since","refills","demand","disk","rate","limit","cooldown","still"]
hash: "a0382bedaad4661ec0fc222b10f0d80c31962a1d7fd48177b1c4387a1102f7ab"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev up","chunkId":"updating#what-restarts"},{"kind":"code","literal":"launchctl kickstart -k","chunkId":"updating#what-restarts"},{"kind":"code","literal":"com.hev.secrets","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hevd.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"systemctl --user restart","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev-secrets.service","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev\\hevd","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev d","chunkId":"updating#what-restarts"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"updating#what-restarts"}]
sources: [{"chunkId":"updating#what-restarts","url":"/docs/updating#what-restarts","anchor":"what-restarts"}]
---

An update restarts hevd and, if installed and running, the standalone secrets cache service on macOS via launchd and on Linux via systemd user units, and the hevd scheduled task on Windows, checking each comes back up. Hand-started daemons, dispatched runs and the dashboard container are not restarted. Restarting the secrets cache sends no request, since it refills on demand and an on-disk rate-limit cooldown still applies.
