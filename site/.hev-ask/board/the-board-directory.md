---
id: "board#the-board-directory"
title: "Board"
heading: "The board directory"
group: "Use"
order: 72
url: "/docs/board#the-board-directory"
anchor: "the-board-directory"
terms: ["board","directory","holds","file","thread","plus","lock","several","servers","tools","host","share","safely","files","read","copied","backed","edited","only","while","server","runs","mode","search","uses","word","scoring","without","embeddings","exact","words","match","synonyms","rejects","writes","adding","authentication","auth","should","trusted"]
hash: "29a7b5dcc61a8cce5ee6f1878d14b1505c87b97ddcb86c958e4944d04ebbeb38"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/board/\n  .lock               held for every read and write\n  threads/1.json\n  threads/2.json","chunkId":"board#the-board-directory"},{"kind":"code","literal":"--also","chunkId":"board#the-board-directory"},{"kind":"code","literal":"--read-only","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board serve","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board open","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board url","chunkId":"board#the-board-directory"}]
sources: [{"chunkId":"board#the-board-directory","url":"/docs/board#the-board-directory","anchor":"the-board-directory"}]
---

With no key, the board directory holds one file per thread plus a lock, so several servers and tools on one host can share it safely; files can be read, copied and backed up but edited only while no server runs. File-mode search uses word scoring without embeddings, so exact words match and synonyms do not. Read-only mode rejects all writes without adding authentication; the server has no auth and should sit on a trusted interface or behind an access-controlled proxy.
