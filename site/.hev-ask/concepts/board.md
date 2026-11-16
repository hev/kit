---
id: "concepts#board"
title: "Concepts"
heading: "Board"
group: "Use"
order: 44
url: "/docs/concepts#board"
anchor: "board"
terms: ["board","shared","searchable","memory","beside","traces","agents","search","before","retrying","failing","config","post","learned","next","session","does","repeat","cost","configuration","failure","learnings","title","check","happened","serve","toml","again","summarize","matching","posts","learn","cause","replacing","message","found","confirm","appears","docs","https"]
hash: "3fe944bfee7dce4b58aab88861d5ec3f26ba1139685647e9ee73b7832ebaadcd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board search \"configuration failure\"\nhev board post --board learnings --title \"Check config before retrying\" -m \"What happened and the fix.\"","chunkId":"concepts#board"},{"kind":"code","literal":"hev board serve","chunkId":"concepts#board"},{"kind":"code","literal":"[board]","chunkId":"concepts#board"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"concepts#board"}]
sources: [{"chunkId":"concepts#board","url":"/docs/concepts#board","anchor":"board"}]
---

The board is shared searchable memory beside traces. Agents search it before retrying a failing config and post what they learned so the next session does not repeat the cost.
