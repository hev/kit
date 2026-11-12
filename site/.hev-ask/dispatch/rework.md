---
id: "dispatch#rework"
title: "Dispatch"
heading: "Rework"
group: "Use"
order: 92
url: "/docs/dispatch#rework"
anchor: "rework"
terms: ["rework","relaunches","stopped","same","worktree","branch","note","appended","order","refusing","running","runs","archiving","previous","attempt","files","timestamps","rechecking","capacity","admission","frontmatter","reread","host","harness","model","timeout","stay","dispatched","dispatch","rate","limit","headers","test","asserts","wrong","header","name","stop","result","json"]
hash: "a0372627eec563717701de8431039db07e737e8885a1dc74ffe1ee892a51611e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rework api-rate-limit-headers 'The 429 test asserts on the wrong header name'","chunkId":"dispatch#rework"},{"kind":"code","literal":"stop","chunkId":"dispatch#rework"},{"kind":"code","literal":"## Rework","chunkId":"dispatch#rework"},{"kind":"code","literal":"order.md","chunkId":"dispatch#rework"},{"kind":"code","literal":"result.json","chunkId":"dispatch#rework"},{"kind":"code","literal":"report.md","chunkId":"dispatch#rework"},{"kind":"code","literal":"status.md","chunkId":"dispatch#rework"},{"kind":"code","literal":"log","chunkId":"dispatch#rework"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#rework"}]
sources: [{"chunkId":"dispatch#rework","url":"/docs/dispatch#rework","anchor":"rework"}]
---

Rework relaunches a stopped run on the same worktree and branch with a note appended to its order, refusing running runs, archiving previous attempt files with timestamps, and rechecking capacity and admission. Frontmatter is not reread, so host, harness, model and timeout stay as dispatched.
