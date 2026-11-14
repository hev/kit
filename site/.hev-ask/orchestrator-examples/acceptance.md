---
id: "orchestrator-examples#acceptance"
title: "Orchestrator examples"
heading: "Acceptance"
group: "Use"
order: 207
url: "/docs/orchestrator-examples#acceptance"
anchor: "acceptance"
terms: ["acceptance","template","details","intake","example","order","must","name","render","inside","spec","managed","scope","reference","issue","title","refuses","rendered","values","containing","line","breaks","rendering","cannot","frontmatter","keys","change","host","model","repo","agentprompt","echo","kind","https","linear","fields","status","agent","ready","empty"]
hash: "cd222ef99a36690da19441d939ef40e1782ba5bebdfc0f3e6a3be96fd49d2163"
mode: "agent-primary"
facts: [{"kind":"code","literal":"\u003cAgentPrompt\u003e","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"echo '{\"id\":\"ABC-12\",\"kind\":\"issue\",\"url\":\"https://linear.app/example/issue/ABC-12\",\"fields\":{\"status\":\"agent-ready\",\"title\":\"Empty search shows a blank page\"}}' | hev orchestrate render intake --event -","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"{{.RunName}}","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"manages","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#acceptance"},{"kind":"code","literal":"ABC-*","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"fix.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"intake.md","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"hevkit.com","chunkId":"orchestrator-examples#acceptance"},{"kind":"value","literal":"llms.txt","chunkId":"orchestrator-examples#acceptance"}]
sources: [{"chunkId":"orchestrator-examples#acceptance","url":"/docs/orchestrator-examples#acceptance","anchor":"acceptance"}]
---

Acceptance and template details for the intake example's order template: it is a Go template that must use the run name, render inside the spec's managed scope, and reference the issue's ID, title and URL. kit refuses rendered values containing line breaks and rendering cannot add frontmatter keys, so an issue title cannot change host, model or repo.
