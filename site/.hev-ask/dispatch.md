---
id: "dispatch"
title: "Dispatch"
heading: null
group: "Use"
order: 186
url: "/docs/dispatch"
anchor: null
terms: ["dispatch","sends","order","host","starts","worker","worktree","fresh","branch","under","supervisor","owns","timeout","result","state","lives","orders","folder","inspected","anywhere","same","config","workers","codex","default","claude","needs","daemon","traces","tagged","name","harness","reads","shelf","skills","output","format","steps","astro","linkgrid"]
hash: "eeef1eb0f77343f2c46b1b7eaecda44cc259ac6e026593586a7219babddc0756"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch","chunkId":"dispatch"},{"kind":"code","literal":"~/.hev/orders/\u003cname\u003e/","chunkId":"dispatch"},{"kind":"code","literal":"~/dispatch/","chunkId":"dispatch"},{"kind":"code","literal":"~/.hev/orders/","chunkId":"dispatch"},{"kind":"code","literal":"HEV_ORDERS_DIR","chunkId":"dispatch"},{"kind":"code","literal":"codex","chunkId":"dispatch"},{"kind":"code","literal":"claude","chunkId":"dispatch"},{"kind":"code","literal":"agy","chunkId":"dispatch"},{"kind":"code","literal":"harness","chunkId":"dispatch"},{"kind":"code","literal":"and reads shelf skills from","chunkId":"dispatch"},{"kind":"flag","literal":"--output-format","chunkId":"dispatch"},{"kind":"value","literal":"Steps.astro","chunkId":"dispatch"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"dispatch"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"dispatch"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"dispatch"},{"kind":"value","literal":"Callout.astro","chunkId":"dispatch"},{"kind":"value","literal":"Edition.astro","chunkId":"dispatch"},{"kind":"value","literal":"0.7","chunkId":"dispatch"}]
sources: [{"chunkId":"dispatch","url":"/docs/dispatch","anchor":null}]
---

Dispatch sends an order to a host, which starts one worker in its own git worktree on a fresh branch under a supervisor that owns timeout and result. All run state lives in the host's orders folder and can be inspected anywhere with the same dispatch config. Workers are codex (default), claude or agy; dispatch needs no daemon and traces are tagged with the order name.
