---
id: "steering#levels"
title: "Steering"
heading: "Levels"
group: "Use"
order: 343
url: "/docs/steering#levels"
anchor: "levels"
terms: ["levels","four","steering","note","waits","agent","next","turn","urgent","lands","tool","call","blocks","writes","commits","pushes","dispatches","until","acknowledges","while","reads","keep","working","halt","every","ends","redirect","halts","checkpoints","worktree","reworks","same","message","keeping","branch","dispatch","runs","only","nothing","killed"]
hash: "3c2701af835fc18c705bc191ea0feaa40ea00b0fba098d2ebe95cd6e86c0484a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"note","chunkId":"steering#levels"},{"kind":"code","literal":"dispatch say","chunkId":"steering#levels"},{"kind":"code","literal":"orchestrate say","chunkId":"steering#levels"},{"kind":"code","literal":"peers send","chunkId":"steering#levels"},{"kind":"code","literal":"urgent","chunkId":"steering#levels"},{"kind":"code","literal":"hev steer ack ID","chunkId":"steering#levels"},{"kind":"code","literal":"hev dispatch","chunkId":"steering#levels"},{"kind":"code","literal":"gh pr","chunkId":"steering#levels"},{"kind":"code","literal":"halt","chunkId":"steering#levels"},{"kind":"code","literal":"halting","chunkId":"steering#levels"},{"kind":"code","literal":"halted","chunkId":"steering#levels"},{"kind":"code","literal":"redirect","chunkId":"steering#levels"},{"kind":"code","literal":"--halt-timeout","chunkId":"steering#levels"},{"kind":"code","literal":"[hooks] halt_timeout","chunkId":"steering#levels"}]
sources: [{"chunkId":"steering#levels","url":"/docs/steering#levels","anchor":"levels"}]
---

Four steering levels: a note waits for the agent's next turn; urgent lands at the next tool call and blocks writes, commits, pushes, dispatches and PR writes until the agent acknowledges, while reads keep working; halt blocks every tool call so the agent ends its turn; redirect halts, checkpoints the worktree and reworks the same run with your message, keeping branch and PR, for dispatch runs only. Nothing is killed to deliver a message, but a halted run that has not ended its turn after ten minutes is hard-stopped.
