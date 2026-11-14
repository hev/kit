---
id: "dispatch#talk-to-a-running-worker"
title: "Dispatch"
heading: "Talk to a running worker"
group: "Use"
order: 151
url: "/docs/dispatch#talk-to-a-running-worker"
anchor: "talk-to-a-running-worker"
terms: ["talk","running","worker","messaging","live","appends","timestamped","note","inbox","checks","between","steps","treats","instructions","without","interrupting","current","step","ended","starts","rework","dispatch","rate","limit","headers","also","cover","path","diff","send","replace","name","instruction","confirm","message","show","prints","tell","first","because"]
hash: "dd5855b24da99e19796bcb8175f8ebe8dbef2f15362b1591cc8b19511a47e1f2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch say api-rate-limit-headers 'Also cover the 503 path'\ngit diff | hev dispatch say api-rate-limit-headers -","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"say","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#talk-to-a-running-worker"}]
sources: [{"chunkId":"dispatch#talk-to-a-running-worker","url":"/docs/dispatch#talk-to-a-running-worker","anchor":"talk-to-a-running-worker"}]
---

Messaging a live run appends a timestamped note to its inbox, which the worker checks between steps and treats as instructions without interrupting the current step; messaging an ended run starts rework with that note.
