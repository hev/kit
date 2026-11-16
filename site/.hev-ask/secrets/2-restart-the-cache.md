---
id: "secrets#2-restart-the-cache"
title: "Secrets"
heading: "2. Restart the cache"
group: "Operate"
order: 251
url: "/docs/secrets#2-restart-the-cache"
anchor: "2-restart-the-cache"
terms: ["restart","cache","second","step","1password","setup","hevd","serves","account","default","token","file","check","ping","shows","linux","under","systemd","macos","launchd","windows","scheduled","task","systemctl","user","service","daemon","stop","unloads","loads","again","secrets","start","another","drop","switch","first","powershell"]
hash: "351538b63139260ca0752f16e9db2a0c8fd9f8d6bc914a205b17b1a299861e40"
mode: "agent-primary"
facts: [{"kind":"code","literal":"systemctl --user restart hevd   # Linux (systemd user service)\nhev daemon stop \u0026\u0026 hev up       # macOS: stop unloads the launchd job, up loads it again\nhev secrets ping","chunkId":"secrets#2-restart-the-cache"},{"kind":"code","literal":"\\hev\\hevd","chunkId":"secrets#2-restart-the-cache"},{"kind":"code","literal":"hev daemon stop; hev daemon start","chunkId":"secrets#2-restart-the-cache"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#2-restart-the-cache"}]
sources: [{"chunkId":"secrets#2-restart-the-cache","url":"/docs/secrets#2-restart-the-cache","anchor":"2-restart-the-cache"}]
---

Second step of 1Password setup: restart hevd so it serves the account from the default token file, then check with a ping. Shows how to restart on Linux under systemd, on macOS under launchd, and on Windows via the scheduled task.
