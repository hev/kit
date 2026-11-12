---
id: "orchestrator-examples#run-it"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 174
url: "/docs/orchestrator-examples#run-it"
anchor: "run-it"
terms: ["intake","orchestrator","first","rounds","render","orders","record","receipts","without","starting","runs","preview","rule","against","sample","event","switch","live","reapply","watch","through","dispatch","listings","round","logs","pause","file","remove","labels","issues","want","started","before","going","orchestrate","apply","check","spec","writes","nothing"]
hash: "241e2b04e4042cd748fa6023ba7f770c0c4f4c92a530efd5f7de6731538dd01a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f intake.md --dry-run   # check the spec; writes nothing\nhev orchestrate apply -f intake.md             # start the loop, rule in dry-run\nhev orchestrate show intake                    # state, rounds, intake signals\nls ~/dispatch/orch-intake/rounds/001/          # the orders it would have sent","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' \\\n  | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"hev orchestrate apply -f intake.md\nhev dispatch ls                    # runs it started show intake in the ORCH column\nhev orchestrate logs intake --round 4\ntouch PAUSED                       # beside intake.md: no polls, no dispatches\nrm PAUSED                          # resumes promptly\nhev orchestrate stop intake","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: live","chunkId":"orchestrator-examples#run-it"}]
sources: [{"chunkId":"orchestrator-examples#run-it","url":"/docs/orchestrator-examples#run-it","anchor":"run-it"}]
---

Run the intake orchestrator first in dry-run so rounds render orders and record receipts without starting runs, preview a rule against a sample event, then switch the rule to live and reapply. Watch it through dispatch listings and round logs, pause with a pause file, and remove labels from issues you do not want started before going live.
