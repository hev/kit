---
id: "orchestrator-examples#run-it"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 179
url: "/docs/orchestrator-examples#run-it"
anchor: "run-it"
terms: ["start","intake","orchestrator","first","rounds","render","orders","record","receipts","nothing","apply","check","real","inspect","state","would","sent","also","preview","rule","against","hand","made","event","orchestrate","spec","writes","loop","show","signals","dispatch","orch","echo","kind","issue","https","linear","example","fields","status"]
hash: "241e2b04e4042cd748fa6023ba7f770c0c4f4c92a530efd5f7de6731538dd01a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f intake.md --dry-run   # check the spec; writes nothing\nhev orchestrate apply -f intake.md             # start the loop, rule in dry-run\nhev orchestrate show intake                    # state, rounds, intake signals\nls ~/dispatch/orch-intake/rounds/001/          # the orders it would have sent","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' \\\n  | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"hev orchestrate apply -f intake.md\nhev dispatch ls                    # runs it started show intake in the ORCH column\nhev orchestrate logs intake --round 4\ntouch PAUSED                       # beside intake.md: no polls, no dispatches\nrm PAUSED                          # resumes promptly\nhev orchestrate stop intake","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: live","chunkId":"orchestrator-examples#run-it"}]
sources: [{"chunkId":"orchestrator-examples#run-it","url":"/docs/orchestrator-examples#run-it","anchor":"run-it"}]
---

Start the intake orchestrator in dry-run first, so rounds render orders and record receipts but start nothing. Apply with a dry-run check, then apply for real, and inspect state and the orders it would have sent. You can also preview a rule against a hand-made event.
