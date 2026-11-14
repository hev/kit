---
id: "orchestrator-examples#run-it"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 224
url: "/docs/orchestrator-examples#run-it"
anchor: "run-it"
terms: ["intake","loop","safely","spec","apply","rule","still","rounds","render","orders","receipts","start","nothing","preview","rendering","against","sample","event","switch","live","monitor","started","runs","pause","creating","file","beside","stop","going","starts","every","currently","labelled","issue","budget","unlabel","anything","want","first","orchestrate"]
hash: "241e2b04e4042cd748fa6023ba7f770c0c4f4c92a530efd5f7de6731538dd01a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f intake.md --dry-run   # check the spec; writes nothing\nhev orchestrate apply -f intake.md             # start the loop, rule in dry-run\nhev orchestrate show intake                    # state, rounds, intake signals\nls ~/dispatch/orch-intake/rounds/001/          # the orders it would have sent","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' \\\n  | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"hev orchestrate apply -f intake.md\nhev dispatch ls                    # runs it started show intake in the ORCH column\nhev orchestrate logs intake --round 4\ntouch PAUSED                       # beside intake.md: no polls, no dispatches\nrm PAUSED                          # resumes promptly\nhev orchestrate stop intake","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: dry-run","chunkId":"orchestrator-examples#run-it"},{"kind":"code","literal":"mode: live","chunkId":"orchestrator-examples#run-it"}]
sources: [{"chunkId":"orchestrator-examples#run-it","url":"/docs/orchestrator-examples#run-it","anchor":"run-it"}]
---

How to run the intake loop safely: dry-run the spec, apply with the rule still in dry-run so rounds render orders and receipts but start nothing, preview rendering against a sample event, then switch the rule to live. Monitor started runs, pause by creating a pause file beside the spec, and stop it; going live starts every currently labelled issue up to the budget, so unlabel anything you don't want first.
