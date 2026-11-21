---
id: "failure-modes"
title: "Failure modes"
heading: null
group: "Operate"
order: 217
url: "/docs/failure-modes"
anchor: null
terms: ["goes","wrong","dispatching","failures","appear","early","command","record","retry","loop","inspect","show","logs","find","failed","host","state","error","dispatch","every","name","order","status","report","result","inbox","harness","output","callout","astro","codetabs","agentprompt","work","about","fails","says","there","failure","shows","most"]
hash: "a130bc0e56413fe3581b37ce600d35a9abdee4da9914a047ed75ad7b763a701f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls --all          # every run on every host, with its state and error\nhev dispatch show NAME         # order, status, report, result and inbox\nhev dispatch logs NAME -f      # harness output","chunkId":"failure-modes"},{"kind":"code","literal":"NAME","chunkId":"failure-modes"},{"kind":"code","literal":"HOST","chunkId":"failure-modes"},{"kind":"value","literal":"Callout.astro","chunkId":"failure-modes"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"failure-modes"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"failure-modes"}]
sources: [{"chunkId":"failure-modes","url":"/docs/failure-modes","anchor":null}]
---

What goes wrong when dispatching: failures appear early on the command you ran or in the run's record, with no retry loop. Inspect with ls --all, show and logs to find a failed run's host, state and error.
