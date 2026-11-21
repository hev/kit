---
id: "orchestrator-examples#run-it"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 297
url: "/docs/orchestrator-examples#run-it"
anchor: "run-it"
terms: ["intake","orchestrator","first","validate","spec","apply","rule","still","mode","inspect","show","list","round","folder","orders","would","sent","orchestrate","check","writes","nothing","start","loop","state","rounds","signals","orch","echo","kind","issue","https","linear","example","fields","status","agent","ready","title","empty","search"]
hash: "290a90eea8cbb285b440b294310615011cac90ecaae4fc1604ed8a50644d5b8a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f intake.md --dry-run   # check the spec; writes nothing\nhev orchestrate apply -f intake.md             # start the loop, rule in dry-run\nhev orchestrate show intake                    # state, rounds, intake signals\nls ~/.hev/orders/orch-intake/rounds/001/       # the orders it would have sent","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' \\\n  | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"hev orchestrate apply -f intake.md\nhev dispatch ls                    # runs it started show intake in the ORCH column\nhev orchestrate logs intake --round 4\ntouch PAUSED                       # beside intake.md: no polls, no dispatches\nrm PAUSED                          # resumes promptly\nhev orchestrate stop intake","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: live","chunkId":"orchestrator-examples#run-it"}]
sources: [{"chunkId":"orchestrator-examples#run-it","url":"/docs/orchestrator-examples#run-it","anchor":"run-it"}]
---

Run the intake orchestrator in dry-run first: validate the spec with apply --dry-run, apply it with the rule still in dry-run mode, then inspect with show and list the round folder to see orders it would have sent.
