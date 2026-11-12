---
id: "traces#the-asks-view"
title: "Traces"
heading: "The Asks view"
group: "Use"
order: 285
url: "/docs/traces#the-asks-view"
anchor: "the-asks-view"
terms: ["asks","view","dashboard","lists","traces","newest","first","root","text","caller","time","user","hosts","asked","project","orders","runs","state","cost","duration","filter","host","free","filters","combine","kept","page","filtered","linked","home","mktemp","serve","demo","open","http","8787","show","built","example","data"]
hash: "f84dac24a30ccee3bc8a4af88fcc2134e41289e8497c442bc4c0819773e9dc34"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HOME=$(mktemp -d) hev serve --demo-traces    # then open http://127.0.0.1:8787/#asks","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"Show me the dashboard's Asks view on built-in example data, without dispatching anything: run `HOME=$(mktemp -d) hev serve --demo-traces` and give me http://127.0.0.1:8787/#asks to open. Confirm the server started and the page loads with example traces. Stop the server when I say I am done.\nDocs: https://hevkit.com/docs/traces#the-asks-view (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"requested_by","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"sender","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"local:hev","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"hev","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"laptop → mini, oven","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"ask_user","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"ask_host","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"ask_project","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"continue","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"launch","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"rework","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"retry","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"say","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"amend","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"validate","chunkId":"traces#the-asks-view"}]
sources: [{"chunkId":"traces#the-asks-view","url":"/docs/traces#the-asks-view","anchor":"the-asks-view"}]
---

The dashboard's Asks tab lists traces newest first, with root ask text and caller, time, user, hosts (where asked and where run), project, orders and runs, state, cost and duration. Filter by user, host and project or free text; filters combine and are kept in the page URL so filtered lists can be linked.
