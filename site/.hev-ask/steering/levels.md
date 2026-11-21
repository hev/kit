---
id: "steering#levels"
title: "Steering"
heading: "Levels"
group: "Use"
order: 404
url: "/docs/steering#levels"
anchor: "levels"
terms: ["levels","steering","note","waits","agent","next","turn","dispatch","orchestrate","peers","send","urgent","lands","tool","call","blocks","writes","commits","dispatches","until","acknowledged","stronger","halt","redirect","steer","halting","halted","timeout","hooks","level","happens","notes","once","runs","file","edits","commit","push","blocked","reason"]
hash: "3c2701af835fc18c705bc191ea0feaa40ea00b0fba098d2ebe95cd6e86c0484a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"note","chunkId":"steering#levels"},{"kind":"code","literal":"dispatch say","chunkId":"steering#levels"},{"kind":"code","literal":"orchestrate say","chunkId":"steering#levels"},{"kind":"code","literal":"peers send","chunkId":"steering#levels"},{"kind":"code","literal":"urgent","chunkId":"steering#levels"},{"kind":"code","literal":"hev steer ack ID","chunkId":"steering#levels"},{"kind":"code","literal":"hev dispatch","chunkId":"steering#levels"},{"kind":"code","literal":"gh pr","chunkId":"steering#levels"},{"kind":"code","literal":"halt","chunkId":"steering#levels"},{"kind":"code","literal":"halting","chunkId":"steering#levels"},{"kind":"code","literal":"halted","chunkId":"steering#levels"},{"kind":"code","literal":"redirect","chunkId":"steering#levels"},{"kind":"code","literal":"--halt-timeout","chunkId":"steering#levels"},{"kind":"code","literal":"[hooks] halt_timeout","chunkId":"steering#levels"}]
sources: [{"chunkId":"steering#levels","url":"/docs/steering#levels","anchor":"levels"}]
---

Steering levels: note waits for the agent's next turn (dispatch say, orchestrate say, peers send), urgent lands at the next tool call and blocks writes, commits and dispatches until acknowledged, and stronger levels halt or redirect.
