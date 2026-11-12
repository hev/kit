---
id: "traces#start-capturing"
title: "Traces"
heading: "Start capturing"
group: "Use"
order: 284
url: "/docs/traces#start-capturing"
anchor: "start-capturing"
terms: ["start","capturing","setup","machine","writes","config","installs","hevd","under","launchd","systemd","traces","skill","agent","harnesses","present","without","stay","local","files","exporting","layer","before","sends","sessions","team","shared","archive","saves","separate","commands","daemon","show","status","stop","unload","while","keeping","export","hevlayer"]
hash: "16fba754766f4b3e19a5ea3197173b2a74c5d25b2b3a7cdba8ef423abfb92790"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev-traces","chunkId":"traces#start-capturing"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev init","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev d","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev s","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev stop","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev down","chunkId":"traces#start-capturing"},{"kind":"value","literal":"hevlayer.com","chunkId":"traces#start-capturing"}]
sources: [{"chunkId":"traces#start-capturing","url":"/docs/traces#start-capturing","anchor":"start-capturing"}]
---

Setup on one machine writes config, installs hevd under launchd or systemd, and installs the traces skill for the agent harnesses present. Without a key traces stay in local files; exporting a layer key before setup sends sessions to the team's shared hev layer archive and saves the key in config. Separate commands start the daemon, show status, stop it, or unload it while keeping the archive.
