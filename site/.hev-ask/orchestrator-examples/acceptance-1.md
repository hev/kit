---
id: "orchestrator-examples#acceptance-1"
title: "Orchestrator examples"
heading: "Acceptance"
group: "Use"
order: 218
url: "/docs/orchestrator-examples#acceptance-1"
anchor: "acceptance-1"
terms: ["acceptance","items","gauntlet","example","fixed","empty","state","search","page","judged","against","reference","image","pinned","commit","item","separately","checkable","also","explains","validator","host","harness","model","come","dispatching","validation","config","must","differ","builder","agentprompt","codetabs","dispatches","order","codex","here","dispatch","hosts","port"]
hash: "c934619e502d76a675087109c42e3f7335df1f5edde838c10e3cd1a313024156"
mode: "agent-primary"
facts: [{"kind":"code","literal":"\u003cAgentPrompt\u003e","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"code","literal":"\u003c/AgentPrompt\u003e\n\n\u003c/CodeTabs\u003e\n\nThe validator's harness and model come from the config on the host that\ndispatches. They must differ from the order's (`codex` here):\n\n\u003cCodeTabs\u003e","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"flag","literal":"--dir","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"flag","literal":"--port","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"value","literal":"claude-opus-5-5","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"value","literal":"empty-state-page.md","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"value","literal":"search-empty.png","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"value","literal":"hevkit.com","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"value","literal":"llms.txt","chunkId":"orchestrator-examples#acceptance-1"},{"kind":"value","literal":"dispatch.toml","chunkId":"orchestrator-examples#acceptance-1"}]
sources: [{"chunkId":"orchestrator-examples#acceptance-1","url":"/docs/orchestrator-examples#acceptance-1","anchor":"acceptance-1"}]
---

Acceptance items for the Gauntlet example's fixed bar, an empty-state search page judged against a reference image pinned to a commit, each item separately checkable. It also explains that the validator's host, harness and model come from the dispatching host's validation config and must differ from the builder's.
