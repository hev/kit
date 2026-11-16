---
id: "updating#hev-update"
title: "Updating and channels"
heading: "hev update"
group: "Operate"
order: 394
url: "/docs/updating#hev-update"
anchor: "hev-update"
terms: ["update","reference","command","modes","move","newest","build","host","channel","switch","record","specific","named","only","backwards","check","whether","behind","without","changing","anything","roll","back","last","replaced","toggles","forward","again","repeated","already","reports","changes","nothing","json","report","available","edge","release","version","rollback"]
hash: "d7d4a5ff055db8fa1f6e5ff05ae79b92aabf799f86ceea1f284e6282a6a43233"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev update","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --channel edge","chunkId":"updating#hev-update"},{"kind":"code","literal":"edge","chunkId":"updating#hev-update"},{"kind":"code","literal":"--channel release","chunkId":"updating#hev-update"},{"kind":"code","literal":"release","chunkId":"updating#hev-update"},{"kind":"code","literal":"--to","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --to VERSION","chunkId":"updating#hev-update"},{"kind":"code","literal":"X.Y.Z","chunkId":"updating#hev-update"},{"kind":"code","literal":"X.Y.Z-edge.\u003csha\u003e","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --check","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --rollback","chunkId":"updating#hev-update"},{"kind":"code","literal":"--json","chunkId":"updating#hev-update"}]
sources: [{"chunkId":"updating#hev-update","url":"/docs/updating#hev-update","anchor":"hev-update"}]
---

Reference for the update command's modes: move to the newest build on the host's channel, switch and record the channel, move to a specific named build (the only way to go backwards), check whether the host is behind without changing anything, and roll back to the build the last update replaced, which toggles forward again if repeated. A host already on the newest build reports so and changes nothing, and a JSON report is available.
