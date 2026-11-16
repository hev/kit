---
id: "failure-modes"
title: "Failure modes"
heading: null
group: "Operate"
order: 161
url: "/docs/failure-modes"
anchor: null
terms: ["dispatch","fails","early","says","there","retry","loop","failure","shows","command","record","host","inspected","through","full","listing","details","harness","every","state","error","show","name","order","status","report","result","inbox","logs","output","callout","astro","codetabs","agentprompt","goes","wrong","work","about","most","errors"]
hash: "a130bc0e56413fe3581b37ce600d35a9abdee4da9914a047ed75ad7b763a701f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls --all          # every run on every host, with its state and error\nhev dispatch show NAME         # order, status, report, result and inbox\nhev dispatch logs NAME -f      # harness output","chunkId":"failure-modes"},{"kind":"code","literal":"NAME","chunkId":"failure-modes"},{"kind":"code","literal":"HOST","chunkId":"failure-modes"},{"kind":"value","literal":"Callout.astro","chunkId":"failure-modes"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"failure-modes"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"failure-modes"}]
sources: [{"chunkId":"failure-modes","url":"/docs/failure-modes","anchor":null}]
---

Dispatch fails early and says why: there is no retry loop, so a failure shows on the command you ran or in the run's record. Any run that got as far as a record on the host can be inspected through the full run listing, the run's details and its harness log.
