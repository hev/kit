---
id: "updating#hev-update"
title: "Updating and channels"
heading: "hev update"
group: "Operate"
order: 305
url: "/docs/updating#hev-update"
anchor: "hev-update"
terms: ["update","variants","args","moves","newest","build","channel","edge","release","records","option","named","version","possibly","backwards","check","reports","without","changing","anything","rollback","returns","last","replaced","repeating","goes","forward","again","json","output","available","host","already","current","changes","nothing","command","does","move","record"]
hash: "d7d4a5ff055db8fa1f6e5ff05ae79b92aabf799f86ceea1f284e6282a6a43233"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev update","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --channel edge","chunkId":"updating#hev-update"},{"kind":"code","literal":"edge","chunkId":"updating#hev-update"},{"kind":"code","literal":"--channel release","chunkId":"updating#hev-update"},{"kind":"code","literal":"release","chunkId":"updating#hev-update"},{"kind":"code","literal":"--to","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --to VERSION","chunkId":"updating#hev-update"},{"kind":"code","literal":"X.Y.Z","chunkId":"updating#hev-update"},{"kind":"code","literal":"X.Y.Z-edge.\u003csha\u003e","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --check","chunkId":"updating#hev-update"},{"kind":"code","literal":"hev update --rollback","chunkId":"updating#hev-update"},{"kind":"code","literal":"--json","chunkId":"updating#hev-update"}]
sources: [{"chunkId":"updating#hev-update","url":"/docs/updating#hev-update","anchor":"hev-update"}]
---

hev update variants: no args moves to the newest build on the channel; channel edge or release records a channel; a to option moves to a named version, possibly backwards; check reports without changing anything; rollback returns to the build last replaced, and repeating it goes forward again. JSON output is available, and a host already current changes nothing.
