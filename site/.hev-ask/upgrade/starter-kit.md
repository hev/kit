---
id: "upgrade#starter-kit"
title: "Upgrade"
heading: "Starter Kit"
group: "Start"
order: 359
url: "/docs/upgrade#starter-kit"
anchor: "starter-kit"
terms: ["starter","setup","writes","config","installs","daemon","skills","uses","docker","scrubbed","sessions","local","file","archive","word","index","relocatable","environment","search","says","matches","words","only","never","pruned","about","thousand","node","holds","dashboard","board","serving","machine","toml","hevd","traces","serve","bind","starts","nothing"]
hash: "417f27584b2b8dc1f07772d4d95d01e16cf292be80394ea859c942a4f17acf3a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"hevd","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"[archive] dir","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"hev traces search","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"hev serve --bind","chunkId":"upgrade#starter-kit"}]
sources: [{"chunkId":"upgrade#starter-kit","url":"/docs/upgrade#starter-kit","anchor":"starter-kit"}]
---

Starter Kit setup with no key writes config, installs the daemon and skills, and uses no Docker; scrubbed sessions go to a local file archive with a word index, relocatable via config or environment. Search says it matches words only, the archive is never pruned (about 400 MB per thousand sessions), each node holds only its own sessions, and the dashboard and board are local to the serving machine.
