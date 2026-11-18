---
id: "upgrade#switching-later"
title: "Upgrade"
heading: "Switching later"
group: "Start"
order: 407
url: "/docs/upgrade#switching-later"
anchor: "switching-later"
terms: ["switching","later","changing","tiers","does","move","data","unless","send","signing","adding","starts","empty","shared","archive","leaves","files","place","field","import","sends","machine","file","board","team","early","access","backfill","past","sessions","transcripts","still","disk","mode","readable","original","host","status","commands","tell"]
hash: "7748889c1d1e2cf3c48f40e0cd7c4cc503eb9abde2b04ab88b1398f5d5270c45"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev login --import","chunkId":"upgrade#switching-later"},{"kind":"code","literal":"hev backfill","chunkId":"upgrade#switching-later"},{"kind":"code","literal":"hev daemon status","chunkId":"upgrade#switching-later"},{"kind":"code","literal":"hev board whoami","chunkId":"upgrade#switching-later"}]
sources: [{"chunkId":"upgrade#switching-later","url":"/docs/upgrade#switching-later","anchor":"switching-later"}]
---

Changing tiers does not move data unless you send it: signing in or adding a key later starts an empty shared archive and leaves the files in place. Field Kit has an import that sends a machine's file archive and board to the team (early access), and Pro Kit has a backfill that sends past sessions from transcripts still on disk. The board starts empty in its new mode with its old files still readable on the original host, and status commands tell you where traces go and where the board is kept without reading the key.
