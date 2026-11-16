---
id: "secrets#2-restart-the-cache"
title: "Secrets"
heading: "2. Restart the cache"
group: "Operate"
order: 303
url: "/docs/secrets#2-restart-the-cache"
anchor: "2-restart-the-cache"
terms: ["restart","cache","1password","setup","step","token","default","file","daemon","ping","confirm","serves","account","systemd","user","service","linux","stop","bring","macos","scheduled","task","windows","systemctl","hevd","unloads","launchd","loads","again","secrets","start","check","another","drop","switch","first","powershell"]
hash: "351538b63139260ca0752f16e9db2a0c8fd9f8d6bc914a205b17b1a299861e40"
mode: "agent-primary"
facts: [{"kind":"code","literal":"systemctl --user restart hevd   # Linux (systemd user service)\nhev daemon stop \u0026\u0026 hev up       # macOS: stop unloads the launchd job, up loads it again\nhev secrets ping","chunkId":"secrets#2-restart-the-cache"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"secrets#2-restart-the-cache"},{"kind":"code","literal":"hev daemon stop; hev daemon start","chunkId":"secrets#2-restart-the-cache"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#2-restart-the-cache"}]
sources: [{"chunkId":"secrets#2-restart-the-cache","url":"/docs/secrets#2-restart-the-cache","anchor":"2-restart-the-cache"}]
---

1Password setup, step two: with the token in the default file, restart the daemon and ping to confirm it serves the account, using the systemd user service on Linux, a stop and bring-up on macOS, or the scheduled task on Windows.
