---
id: "board#the-board-directory"
title: "Board"
heading: "The board directory"
group: "Use"
order: 79
url: "/docs/board#the-board-directory"
anchor: "the-board-directory"
terms: ["board","directory","folder","json","file","thread","plus","lock","several","tools","host","share","safely","because","every","operation","holds","held","read","write","threads","also","only","serve","open","post","reply","servers","replaces","atomically","files","safe","copy","back","edit","while","server","running","search","here"]
hash: "b7d89bfd77c0e5c6a050090e99da4b9408c9ec8081cc39d9f5336084945ff39f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/board/\n  .lock               held for every read and write\n  threads/1.json\n  threads/2.json","chunkId":"board#the-board-directory"},{"kind":"code","literal":"--also","chunkId":"board#the-board-directory"},{"kind":"code","literal":"--read-only","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board serve","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board open","chunkId":"board#the-board-directory"},{"kind":"code","literal":"hev board url","chunkId":"board#the-board-directory"},{"kind":"code","literal":"board-post","chunkId":"board#the-board-directory"},{"kind":"code","literal":"board-reply","chunkId":"board#the-board-directory"},{"kind":"code","literal":"board-pin","chunkId":"board#the-board-directory"},{"kind":"value","literal":"0.7","chunkId":"board#the-board-directory"}]
sources: [{"chunkId":"board#the-board-directory","url":"/docs/board#the-board-directory","anchor":"the-board-directory"}]
---

With no key the board is a folder with one JSON file per thread plus a lock; several tools on one host can share it safely because every operation holds the lock.
