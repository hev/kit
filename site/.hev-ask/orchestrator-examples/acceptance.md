---
id: "orchestrator-examples#acceptance"
title: "Orchestrator examples"
heading: "Acceptance"
group: "Use"
order: 287
url: "/docs/orchestrator-examples#acceptance"
anchor: "acceptance"
terms: ["acceptance","intake","example","order","template","name","repo","issue","reply","linking","requiring","change","failing","without","test","passing","suite","draft","worker","stops","reports","unclear","agentprompt","echo","kind","https","linear","fields","status","agent","ready","title","empty","search","shows","blank","page","orchestrate","render","event"]
hash: "cd222ef99a36690da19441d939ef40e1782ba5bebdfc0f3e6a3be96fd49d2163"
mode: "agent-primary"
facts: [{"kind":"code","literal":"\u003cAgentPrompt\u003e","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"{{.RunName}}","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"manages","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"ABC-*","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"fix.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"intake.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"hevkit.com","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"llms.txt","chunkId":"orchestrator-examples#acceptance"}]
sources: [{"chunkId":"orchestrator-examples#acceptance","url":"/docs/orchestrator-examples#acceptance","anchor":"acceptance"}]
---

Intake example acceptance: the fix order template (fix.md) is a Go template with run name, repo, issue ID, reply, linking the issue URL, and acceptance requiring a change with a failing-without-it test, passing suite and a draft PR linking the issue; the worker stops and reports if the issue is unclear or too big.
