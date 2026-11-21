---
id: "dispatch#talk-to-a-running-worker"
title: "Dispatch"
heading: "Talk to a running worker"
group: "Use"
order: 200
url: "/docs/dispatch#talk-to-a-running-worker"
anchor: "talk-to-a-running-worker"
terms: ["talk","running","worker","sends","instruction","appending","timestamped","entry","inbox","file","reads","between","steps","ended","starts","rework","instead","dispatch","rate","limit","headers","also","cover","path","diff","send","replace","name","confirm","message","show","prints","tell","first","because","docs","https","hevkit","pages","llms"]
hash: "dd5855b24da99e19796bcb8175f8ebe8dbef2f15362b1591cc8b19511a47e1f2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch say api-rate-limit-headers 'Also cover the 503 path'\ngit diff | hev dispatch say api-rate-limit-headers -","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"say","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#talk-to-a-running-worker"}]
sources: [{"chunkId":"dispatch#talk-to-a-running-worker","url":"/docs/dispatch#talk-to-a-running-worker","anchor":"talk-to-a-running-worker"}]
---

say sends a new instruction to a running worker by appending a timestamped entry to its inbox file, which the worker reads between steps. On an ended run, say starts a rework instead.
