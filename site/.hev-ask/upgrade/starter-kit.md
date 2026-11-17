---
id: "upgrade#starter-kit"
title: "Upgrade"
heading: "Starter Kit"
group: "Start"
order: 410
url: "/docs/upgrade#starter-kit"
anchor: "starter-kit"
terms: ["starter","setup","writes","config","installs","daemon","skills","starts","nothing","docker","scrubs","secrets","session","local","file","archive","word","index","whose","location","changed","search","dashboard","state","match","words","rather","meaning","pair","natural","language","phrasing","exact","tokens","never","pruned","roughly","sessions","holds","only"]
hash: "417f27584b2b8dc1f07772d4d95d01e16cf292be80394ea859c942a4f17acf3a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"hevd","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"[archive] dir","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"hev traces search","chunkId":"upgrade#starter-kit"},{"kind":"code","literal":"hev serve --bind","chunkId":"upgrade#starter-kit"}]
sources: [{"chunkId":"upgrade#starter-kit","url":"/docs/upgrade#starter-kit","anchor":"starter-kit"}]
---

Starter Kit setup with no key writes the config, installs the daemon and skills, and starts nothing in Docker; the daemon scrubs secrets and writes each session to a local file archive with a word index, whose location can be changed. Search and the dashboard state that they match words rather than meaning, so pair a natural-language phrasing with exact tokens. The archive is never pruned, at roughly 400 MB per 1,000 sessions, and holds only that node's sessions, so reading another node's means serving its dashboard from there; dashboard and board APIs are local to the serving machine.
