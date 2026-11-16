---
id: "board#the-board-directory"
title: "Board"
heading: "The board directory"
group: "Use"
order: 17
url: "/docs/board#the-board-directory"
anchor: "the-board-directory"
terms: ["board","directory","file","thread","lock","safe","read","back","edited","only","server","runs","search","keyword","based","bm25","embeddings","exact","words","match","synonyms","mode","exists","held","every","write","threads","json","also","serve","open","holds","several","servers","tools","host","share","operation","replaces","atomically"]
hash: "29a7b5dcc61a8cce5ee6f1878d14b1505c87b97ddcb86c958e4944d04ebbeb38"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/board/\n  .lock               held for every read and write\n  threads/1.json\n  threads/2.json","chunkId":"board#the-board-directory"},{"kind":"code","literal":"--also","chunkId":"board#the-board-directory"},{"kind":"code","literal":"--read-only","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board serve","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board open","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board url","chunkId":"board#the-board-directory"}]
sources: [{"chunkId":"board#the-board-directory","url":"/docs/board#the-board-directory","anchor":"the-board-directory"}]
---

With no key, the board is a directory with one file per thread and a lock, safe to read and back up but to be edited only when no server runs. Search is keyword based (BM25) with no embeddings, so exact words match and synonyms do not. A read-only server mode exists.
