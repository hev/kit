---
id: "orchestrator-examples#acceptance"
title: "Orchestrator examples"
heading: "Acceptance"
group: "Use"
order: 217
url: "/docs/orchestrator-examples#acceptance"
anchor: "acceptance"
terms: ["acceptance","intake","loop","order","template","requested","change","test","fails","without","passing","suite","draft","linking","issue","whose","rendered","must","stay","inside","orchestrator","managed","scope","values","line","breaks","refused","rendering","cannot","frontmatter","keys","title","host","model","repo","agentprompt","echo","kind","https","linear"]
hash: "cd222ef99a36690da19441d939ef40e1782ba5bebdfc0f3e6a3be96fd49d2163"
mode: "agent-primary"
facts: [{"kind":"code","literal":"\u003cAgentPrompt\u003e","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"{{.RunName}}","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"manages","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"ABC-*","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"fix.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"intake.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"hevkit.com","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"llms.txt","chunkId":"orchestrator-examples#acceptance"}]
sources: [{"chunkId":"orchestrator-examples#acceptance","url":"/docs/orchestrator-examples#acceptance","anchor":"acceptance"}]
---

Acceptance for the intake loop's order template: the requested change with a test that fails without it, a passing suite and a draft PR linking the issue. The template is a Go template whose rendered order must stay inside the orchestrator's managed scope; values with line breaks are refused and rendering cannot add frontmatter keys, so an issue title cannot change host, model or repo.
