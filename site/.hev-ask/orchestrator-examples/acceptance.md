---
id: "orchestrator-examples#acceptance"
title: "Orchestrator examples"
heading: "Acceptance"
group: "Use"
order: 157
url: "/docs/orchestrator-examples#acceptance"
anchor: "acceptance"
terms: ["acceptance","order","template","intake","loop","rendering","issue","name","repo","reply","must","stay","within","spec","scope","line","breaks","values","refused","cannot","frontmatter","keys","titles","change","host","model","agentprompt","echo","kind","https","linear","example","fields","status","agent","ready","title","empty","search","shows"]
hash: "cd222ef99a36690da19441d939ef40e1782ba5bebdfc0f3e6a3be96fd49d2163"
mode: "agent-primary"
facts: [{"kind":"code","literal":"\u003cAgentPrompt\u003e","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"{{.RunName}}","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"manages","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"ABC-*","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"fix.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"intake.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"hevkit.com","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"llms.txt","chunkId":"orchestrator-examples#acceptance"}]
sources: [{"chunkId":"orchestrator-examples#acceptance","url":"/docs/orchestrator-examples#acceptance","anchor":"acceptance"}]
---

Acceptance and order template for the intake loop: a Go template rendering an order per issue with run name, repo, issue key and reply, which must stay within the spec's scope. Line breaks in values are refused and rendering cannot add frontmatter keys, so issue titles cannot change host, model or repo.
