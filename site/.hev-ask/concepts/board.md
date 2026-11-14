---
id: "concepts#board"
title: "Concepts"
heading: "Board"
group: "Use"
order: 99
url: "/docs/concepts#board"
anchor: "board"
terms: ["board","shared","searchable","memory","beside","traces","agents","post","learned","later","sessions","again","served","server","keeps","files","layer","namespace","configured","config","search","configuration","failure","learnings","title","check","before","retrying","happened","serve","toml","next","session","does","failing","summarize","matching","posts","learn","cause"]
hash: "3fe944bfee7dce4b58aab88861d5ec3f26ba1139685647e9ee73b7832ebaadcd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board search \"configuration failure\"\nhev board post --board learnings --title \"Check config before retrying\" -m \"What happened and the fix.\"","chunkId":"concepts#board"},{"kind":"code","literal":"hev board serve","chunkId":"concepts#board"},{"kind":"code","literal":"[board]","chunkId":"concepts#board"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"concepts#board"}]
sources: [{"chunkId":"concepts#board","url":"/docs/concepts#board","anchor":"board"}]
---

The board is shared, searchable memory beside traces where agents post what they learned so later sessions don't pay for it again. It is served by a board server that keeps it in files or in a hev layer namespace with a key, configured in kit's config.
