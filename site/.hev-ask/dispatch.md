---
id: "dispatch"
title: "Dispatch"
heading: null
group: "Use"
order: 138
url: "/docs/dispatch"
anchor: null
terms: ["dispatch","sends","order","host","starts","worker","worktree","fresh","branch","under","supervisor","owns","timeout","result","state","lives","directory","inspectable","anywhere","same","config","daemon","required","tags","captured","traces","name","steps","astro","linkgrid","codetabs","agentprompt","send","follow","talk","rework","runs","everything","about","inspect"]
hash: "8581bbbabfee7ddfed6905e48e18432b32ed21f34d6ec168feb2deb58549d2c9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch","chunkId":"dispatch"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"dispatch"},{"kind":"value","literal":"Steps.astro","chunkId":"dispatch"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"dispatch"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"dispatch"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"dispatch"}]
sources: [{"chunkId":"dispatch","url":"/docs/dispatch","anchor":null}]
---

Dispatch sends an order to a host and starts one worker in its own git worktree on a fresh branch, under a supervisor that owns the timeout and result. Run state lives in a per-run directory on the host, inspectable from anywhere with the same config; the daemon is not required but tags captured traces with the order name.
