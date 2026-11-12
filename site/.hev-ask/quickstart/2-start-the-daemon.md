---
id: "quickstart#2-start-the-daemon"
title: "Quickstart"
heading: "2. Start the daemon"
group: "Start"
order: 230
url: "/docs/quickstart#2-start-the-daemon"
anchor: "2-start-the-daemon"
terms: ["start","daemon","running","setup","command","writes","config","installs","hevd","capture","under","launchd","macos","systemd","user","service","linux","layer","traces","stay","local","files","archive","directory","status","shows","last","scan","indexed","unit","count","error","without","usable","manager","supervisor","write","toml","install","hevlayer"]
hash: "bff52cf8073d98381926a39ede6575cafb01368701ac47dd8f76eb71bd6ae2df"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"Run `hev up` to write ~/.hev/config.toml and install the hevd capture daemon (launchd on macOS, systemd user service on Linux). Do not set HEVLAYER_API_KEY; traces should stay in files under ~/.hev/archive/. Then run `hev s` and confirm it shows a recent scan and no error.\nDocs: https://hevkit.com/docs/quickstart#2-start-the-daemon (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"hev s","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"Check that the hev capture daemon is healthy: run `hev s` and tell me the last scan time, how many units it indexed and the last error, if any. If there is no recent scan on Linux, check whether a systemd user manager is available and say so.\nDocs: https://hevkit.com/docs/quickstart#2-start-the-daemon (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"hevd","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"~/.hev/archive/","chunkId":"quickstart#2-start-the-daemon"},{"kind":"code","literal":"hev d","chunkId":"quickstart#2-start-the-daemon"}]
sources: [{"chunkId":"quickstart#2-start-the-daemon","url":"/docs/quickstart#2-start-the-daemon","anchor":"2-start-the-daemon"}]
---

Running the setup command writes the hev config and installs the hevd capture daemon under launchd on macOS or a systemd user service on Linux; with no layer key, traces stay in local files under the hev archive directory. The status command shows the last scan, indexed unit count and last error, and without a usable systemd user manager you run the daemon under your own supervisor.
