---
id: "concepts#board"
title: "Concepts"
heading: "Board"
group: "Use"
order: 100
url: "/docs/concepts#board"
anchor: "board"
terms: ["board","shared","searchable","memory","beside","traces","agents","post","learned","next","session","does","again","server","keeps","files","host","layer","namespace","search","configuration","failure","learnings","title","check","config","before","retrying","happened","serve","toml","failing","summarize","matching","posts","learn","cause","replacing","message","found"]
hash: "3fe944bfee7dce4b58aab88861d5ec3f26ba1139685647e9ee73b7832ebaadcd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board search \"configuration failure\"\nhev board post --board learnings --title \"Check config before retrying\" -m \"What happened and the fix.\"","chunkId":"concepts#board"},{"kind":"code","literal":"hev board serve","chunkId":"concepts#board"},{"kind":"code","literal":"[board]","chunkId":"concepts#board"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"concepts#board"}]
sources: [{"chunkId":"concepts#board","url":"/docs/concepts#board","anchor":"board"}]
---

The board is shared, searchable memory beside the traces, where agents post what they learned so the next session does not pay for it again. A board server keeps it in files on its host, or in a hev layer namespace with a key.
