---
id: "dispatch#talk-to-a-running-worker"
title: "Dispatch"
heading: "Talk to a running worker"
group: "Use"
order: 102
url: "/docs/dispatch#talk-to-a-running-worker"
anchor: "talk-to-a-running-worker"
terms: ["talk","running","worker","subcommand","sends","instruction","lands","inbox","file","read","message","standard","input","dispatch","rate","limit","headers","also","cover","path","diff","send","replace","name","confirm","show","prints","ended","tell","first","because","starts","rework","docs","https","hevkit","pages","llms","live","appends"]
hash: "dd5855b24da99e19796bcb8175f8ebe8dbef2f15362b1591cc8b19511a47e1f2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch say api-rate-limit-headers 'Also cover the 503 path'\ngit diff | hev dispatch say api-rate-limit-headers -","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"say","chunkId":"dispatch#talk-to-a-running-worker"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#talk-to-a-running-worker"}]
sources: [{"chunkId":"dispatch#talk-to-a-running-worker","url":"/docs/dispatch#talk-to-a-running-worker","anchor":"talk-to-a-running-worker"}]
---

The say subcommand sends a running worker a new instruction, which lands in its inbox file, and can read the message from standard input.
