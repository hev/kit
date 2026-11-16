---
id: "dispatch#talk-to-a-running-worker"
title: "Dispatch"
heading: "Talk to a running worker"
group: "Use"
order: 158
url: "/docs/dispatch#talk-to-a-running-worker"
anchor: "talk-to-a-running-worker"
terms: ["talk","running","worker","message","live","appended","inbox","timestamp","checks","between","steps","treats","entries","instructions","does","interrupt","step","progress","come","standard","input","ended","starts","rework","note","dispatch","rate","limit","headers","also","cover","path","diff","send","replace","name","instruction","confirm","show","prints"]
hash: "dd5855b24da99e19796bcb8175f8ebe8dbef2f15362b1591cc8b19511a47e1f2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch say api-rate-limit-headers 'Also cover the 503 path'\ngit diff | hev dispatch say api-rate-limit-headers -","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"say","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#talk-to-a-running-worker"}]
sources: [{"chunkId":"dispatch#talk-to-a-running-worker","url":"/docs/dispatch#talk-to-a-running-worker","anchor":"talk-to-a-running-worker"}]
---

A message to a live run is appended to its inbox with a timestamp; the worker checks the inbox between steps and treats entries as instructions, so it does not interrupt the step in progress. The message can come from standard input. A message to a run that has ended starts a rework with the message as its note.
