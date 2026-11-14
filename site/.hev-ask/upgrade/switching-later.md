---
id: "upgrade#switching-later"
title: "Upgrade"
heading: "Switching later"
group: "Start"
order: 360
url: "/docs/upgrade#switching-later"
anchor: "switching-later"
terms: ["switching","later","changing","tiers","doesn","move","data","unless","send","signing","adding","starts","empty","shared","archive","field","import","machine","file","board","backfill","past","sessions","transcripts","disk","mode","status","commands","show","traces","without","reading","login","daemon","whoami","does","change","leaves","files","sends"]
hash: "7748889c1d1e2cf3c48f40e0cd7c4cc503eb9abde2b04ab88b1398f5d5270c45"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev login --import","chunkId":"upgrade#switching-later"},{"kind":"code","literal":"hev backfill","chunkId":"upgrade#switching-later"},{"kind":"code","literal":"hev daemon status","chunkId":"upgrade#switching-later"},{"kind":"code","literal":"hev board whoami","chunkId":"upgrade#switching-later"}]
sources: [{"chunkId":"upgrade#switching-later","url":"/docs/upgrade#switching-later","anchor":"switching-later"}]
---

Changing tiers doesn't move data unless you send it: signing in or adding a key starts an empty shared archive. Field Kit can import a machine's file archive and board; Pro Kit can backfill past sessions from transcripts on disk. The board starts empty in its new mode, and status commands show where traces and the board go without reading the key.
