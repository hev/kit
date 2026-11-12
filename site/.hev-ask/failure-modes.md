---
id: "failure-modes"
title: "Failure modes"
heading: null
group: "Operate"
order: 105
url: "/docs/failure-modes"
anchor: null
terms: ["guide","goes","wrong","dispatching","failures","show","early","command","record","there","retry","loop","errors","printed","shown","runs","inspected","dispatch","every","host","state","error","name","order","status","report","result","inbox","logs","harness","output","callout","astro","codetabs","agentprompt","work","about","fails","says","failure"]
hash: "a130bc0e56413fe3581b37ce600d35a9abdee4da9914a047ed75ad7b763a701f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls --all          # every run on every host, with its state and error\nhev dispatch show NAME         # order, status, report, result and inbox\nhev dispatch logs NAME -f      # harness output","chunkId":"failure-modes"},{"kind":"code","literal":"NAME","chunkId":"failure-modes"},{"kind":"code","literal":"HOST","chunkId":"failure-modes"},{"kind":"value","literal":"Callout.astro","chunkId":"failure-modes"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"failure-modes"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"failure-modes"}]
sources: [{"chunkId":"failure-modes","url":"/docs/failure-modes","anchor":null}]
---

A guide to what goes wrong when dispatching: failures show up early on the command or in the run record, there is no retry loop, and errors are printed as shown. Runs with a record can be inspected.
