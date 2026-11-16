---
id: "quickstart#2-start-the-daemon"
title: "Quickstart"
heading: "2. Start the daemon"
group: "Start"
order: 290
url: "/docs/quickstart#2-start-the-daemon"
anchor: "2-start-the-daemon"
terms: ["start","daemon","step","brings","machine","writes","config","installs","capture","under","launchd","systemd","user","service","traces","stay","local","archive","files","check","status","recent","scan","units","indexed","error","linux","without","manager","supervisor","write","toml","install","hevd","macos","hevlayer","should","confirm","shows","docs"]
hash: "bff52cf8073d98381926a39ede6575cafb01368701ac47dd8f76eb71bd6ae2df"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"Run `hev up` to write ~/.hev/config.toml and install the hevd capture daemon (launchd on macOS, systemd user service on Linux). Do not set HEVLAYER_API_KEY; traces should stay in files under ~/.hev/archive/. Then run `hev s` and confirm it shows a recent scan and no error.\nDocs: https://hevkit.com/docs/quickstart#2-start-the-daemon (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"hev s","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"Check that the hev capture daemon is healthy: run `hev s` and tell me the last scan time, how many units it indexed and the last error, if any. If there is no recent scan on Linux, check whether a systemd user manager is available and say so.\nDocs: https://hevkit.com/docs/quickstart#2-start-the-daemon (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"quickstart#2-start-the-daemon"}]
sources: [{"chunkId":"quickstart#2-start-the-daemon","url":"/docs/quickstart#2-start-the-daemon","anchor":"2-start-the-daemon"}]
---

Step two brings the machine up, which writes the config and installs the capture daemon under launchd or a systemd user service; with no key, traces stay in local archive files. Check status for a recent scan, units indexed and no error, and on Linux without a user manager run the daemon under your own supervisor.
