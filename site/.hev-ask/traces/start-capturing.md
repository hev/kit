---
id: "traces#start-capturing"
title: "Traces"
heading: "Start capturing"
group: "Use"
order: 315
url: "/docs/traces#start-capturing"
anchor: "start-capturing"
terms: ["start","capturing","running","machine","writes","config","installs","hevd","under","launchd","systemd","traces","skill","supported","agent","harnesses","without","layer","stay","local","files","docker","exporting","first","sends","sessions","team","shared","archive","saved","later","runs","past","backfilled","daemon","status","stop","unload","commands","listed"]
hash: "204c80ed75ba40177ef461cb83488cd24b8e24f1c7284a21bd72e6409b02b8f2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev-traces","chunkId":"traces#start-capturing"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev backfill","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev init","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev d","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev s","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev stop","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev down","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev daemon status","chunkId":"traces#start-capturing"},{"kind":"value","literal":"hevlayer.com","chunkId":"traces#start-capturing"}]
sources: [{"chunkId":"traces#start-capturing","url":"/docs/traces#start-capturing","anchor":"start-capturing"}]
---

Running hev up on a machine writes the config, installs hevd under launchd or systemd and installs the traces skill for supported agent harnesses. Without a hev layer key traces stay in local files with no Docker; exporting a key first sends sessions to the team's shared hev layer archive, the key is saved for later runs, and past sessions can be backfilled. Daemon start, status, stop and unload commands are listed, and unloading keeps the archive.
