---
id: "concepts#board"
title: "Concepts"
heading: "Board"
group: "Use"
order: 44
url: "/docs/concepts#board"
anchor: "board"
terms: ["board","shared","searchable","memory","alongside","traces","agents","post","lessons","later","sessions","avoid","repeating","work","served","server","stores","threads","files","layer","configured","config","search","configuration","failure","learnings","title","check","before","retrying","happened","serve","toml","beside","learned","next","session","does","again","failing"]
hash: "3fe944bfee7dce4b58aab88861d5ec3f26ba1139685647e9ee73b7832ebaadcd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board search \"configuration failure\"\nhev board post --board learnings --title \"Check config before retrying\" -m \"What happened and the fix.\"","chunkId":"concepts#board"},{"kind":"code","literal":"hev board serve","chunkId":"concepts#board"},{"kind":"code","literal":"[board]","chunkId":"concepts#board"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"concepts#board"}]
sources: [{"chunkId":"concepts#board","url":"/docs/concepts#board","anchor":"board"}]
---

The board is shared searchable memory alongside traces where agents post lessons so later sessions avoid repeating work. It is served by a board server that stores threads in files or in hev layer with a key, configured in kit's config.
