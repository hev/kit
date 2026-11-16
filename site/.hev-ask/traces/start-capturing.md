---
id: "traces#start-capturing"
title: "Traces"
heading: "Start capturing"
group: "Use"
order: 373
url: "/docs/traces#start-capturing"
anchor: "start-capturing"
terms: ["start","capturing","setup","command","configures","capture","machine","writes","config","installs","daemon","system","service","traces","skill","whichever","harnesses","stay","files","nothing","runs","docker","exporting","layer","before","sends","sessions","shared","team","archive","saved","adding","later","starts","empty","existing","file","until","backfill","there"]
hash: "28acf59f2464446f2b833a0616dfabe92e9c8a3b1a679248209dd25e379e575c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev-traces","chunkId":"traces#start-capturing"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev backfill","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev init","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev d","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev s","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev stop","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev down","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev daemon status","chunkId":"traces#start-capturing"},{"kind":"value","literal":"hevlayer.com","chunkId":"traces#start-capturing"}]
sources: [{"chunkId":"traces#start-capturing","url":"/docs/traces#start-capturing","anchor":"start-capturing"}]
---

One setup command configures capture on a machine: it writes the config, installs the daemon as a system service and installs the traces skill for whichever harnesses you have. With no key, traces stay in files on that machine and nothing runs in Docker; exporting a hev layer key before setup sends sessions to a shared team archive, and the key is saved in the config. Adding a key later starts an empty shared archive, existing file sessions stay put until you backfill them, and there are commands to start, stop, unload and check the status of the daemon.
