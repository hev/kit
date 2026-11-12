---
id: "failure-modes"
title: "Failure modes"
heading: null
group: "Operate"
order: 99
url: "/docs/failure-modes"
anchor: null
terms: ["dispatch","fails","early","says","retry","loop","errors","appear","command","record","recorded","inspected","through","listing","details","logs","every","host","state","error","show","name","order","status","report","result","inbox","harness","output","callout","astro","codetabs","agentprompt","goes","wrong","work","about","there","failure","shows"]
hash: "a130bc0e56413fe3581b37ce600d35a9abdee4da9914a047ed75ad7b763a701f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls --all          # every run on every host, with its state and error\nhev dispatch show NAME         # order, status, report, result and inbox\nhev dispatch logs NAME -f      # harness output","chunkId":"failure-modes"},{"kind":"code","literal":"NAME","chunkId":"failure-modes"},{"kind":"code","literal":"HOST","chunkId":"failure-modes"},{"kind":"value","literal":"Callout.astro","chunkId":"failure-modes"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"failure-modes"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"failure-modes"}]
sources: [{"chunkId":"failure-modes","url":"/docs/failure-modes","anchor":null}]
---

Dispatch fails early and says why with no retry loop, so errors appear on the command or in the run record. Any recorded run can be inspected through listing, details and logs.
