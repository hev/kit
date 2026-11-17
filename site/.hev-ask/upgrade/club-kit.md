---
id: "upgrade#club-kit"
title: "Upgrade"
heading: "Club Kit"
group: "Start"
order: 408
url: "/docs/upgrade#club-kit"
anchor: "club-kit"
terms: ["club","early","access","open","sign","describes","work","create","join","team","machine","through","browser","stores","privately","works","only","against","machines","never","receive","storage","endpoint","once","signed","daemon","writes","session","locally","first","uploads","outbox","waits","network","down","while","search","board","dashboards","shared"]
hash: "35fcc8715b80084d28b679acb45e978bf50a553bd3c055c3c83da4bb385025ba"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev login\nhev login status","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev login --import","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev login","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hk_…","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev login status","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev logout","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hevd","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev traces search","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev find","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev serve","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev board","chunkId":"upgrade#club-kit"},{"kind":"code","literal":"hev open","chunkId":"upgrade#club-kit"},{"kind":"value","literal":"hevkit.com","chunkId":"upgrade#club-kit"}]
sources: [{"chunkId":"upgrade#club-kit","url":"/docs/upgrade#club-kit","anchor":"club-kit"}]
---

Club Kit is in early access and not yet open for sign-up; this describes how it will work. You create or join a team, then sign each machine in through the browser, which stores a team kit key privately; that key works only against kit's own API and machines never receive a storage endpoint or key. Once signed in, the daemon writes each session locally first and uploads through an outbox that waits if the network is down, while search, the board and the dashboards use the team's shared archive across every node, with sessions and posts stamped by author and machine and deletable only by their owner. Billing is for storage and searches with no per-seat or per-node charge, and existing local sessions and board are only copied to the team by an explicit, resumable import that does not duplicate history.
