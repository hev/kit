---
id: "traces#start-capturing"
title: "Traces"
heading: "Start capturing"
group: "Use"
order: 340
url: "/docs/traces#start-capturing"
anchor: "start-capturing"
terms: ["start","capturing","setup","machine","writes","config","installs","daemon","under","launchd","systemd","traces","skill","without","sessions","local","file","archive","nothing","docker","exporting","layer","first","sends","shared","team","saves","bringing","turbopuffer","advanced","option","there","commands","check","stop","unload","while","keeping","export","hevlayer"]
hash: "16fba754766f4b3e19a5ea3197173b2a74c5d25b2b3a7cdba8ef423abfb92790"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev-traces","chunkId":"traces#start-capturing"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev init","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev d","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev s","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev stop","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev down","chunkId":"traces#start-capturing"},{"kind":"value","literal":"hevlayer.com","chunkId":"traces#start-capturing"}]
sources: [{"chunkId":"traces#start-capturing","url":"/docs/traces#start-capturing","anchor":"start-capturing"}]
---

Setup on a machine writes config, installs the daemon under launchd or systemd, and installs the traces skill. Without a key, sessions go to a local file archive with nothing in Docker; exporting a hev layer key first sends them to a shared team archive and saves the key in config. Bringing your own turbopuffer is an advanced option, and there are commands to start, check, stop or unload the daemon while keeping the archive.
