---
id: "dispatch"
title: "Dispatch"
heading: null
group: "Use"
order: 145
url: "/docs/dispatch"
anchor: null
terms: ["dispatch","sends","order","host","starts","worker","there","worktree","fresh","branch","under","supervisor","owns","timeout","result","everything","about","lives","directory","inspected","machine","same","config","does","need","daemon","traces","captured","tagged","name","also","capturing","steps","astro","linkgrid","codetabs","agentprompt","send","follow","talk"]
hash: "8581bbbabfee7ddfed6905e48e18432b32ed21f34d6ec168feb2deb58549d2c9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch","chunkId":"dispatch"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"dispatch"},{"kind":"value","literal":"Steps.astro","chunkId":"dispatch"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"dispatch"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"dispatch"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"dispatch"}]
sources: [{"chunkId":"dispatch","url":"/docs/dispatch","anchor":null}]
---

Dispatch sends an order to a host and starts one worker there, in its own git worktree on a fresh branch under a supervisor that owns the timeout and the result. Everything about the run lives in a directory on the host and can be inspected from any machine with the same dispatch config. Dispatch does not need the daemon; traces are captured and tagged with the order name if the host is also capturing.
