---
id: "traces#the-asks-view"
title: "Traces"
heading: "The Asks view"
group: "Use"
order: 378
url: "/docs/traces#the-asks-view"
anchor: "the-asks-view"
terms: ["asks","view","dashboard","lists","traces","newest","first","showing","root","text","caller","asked","dispatched","asking","executing","hosts","project","counts","state","cost","duration","list","filters","user","host","kept","page","filtered","linked","opening","trace","shows","bottom","waterfall","grouped","orders","attempts","validations","sessions","under"]
hash: "f84dac24a30ccee3bc8a4af88fcc2134e41289e8497c442bc4c0819773e9dc34"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HOME=$(mktemp -d) hev serve --demo-traces    # then open http://127.0.0.1:8787/#asks","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"Show me the dashboard's Asks view on built-in example data, without dispatching anything: run `HOME=$(mktemp -d) hev serve --demo-traces` and give me http://127.0.0.1:8787/#asks to open. Confirm the server started and the page loads with example traces. Stop the server when I say I am done.\nDocs: https://hevkit.com/docs/traces#the-asks-view (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"requested_by","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"sender","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"local:hev","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"hev","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"laptop → mini, oven","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"ask_user","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"ask_host","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"ask_project","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"continue","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"launch","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"rework","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"retry","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"say","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"amend","chunkId":"traces#the-asks-view"},{"kind":"code","literal":"validate","chunkId":"traces#the-asks-view"}]
sources: [{"chunkId":"traces#the-asks-view","url":"/docs/traces#the-asks-view","anchor":"the-asks-view"}]
---

The dashboard's Asks tab lists traces newest first, showing each root ask's text and caller, when it was asked, who it was dispatched for, the asking and executing hosts, the project, and counts, state, cost and duration. The list filters by user, host, project and text, with filters kept in the page URL so a filtered list can be linked. Opening a trace shows a top-to-bottom waterfall grouped by ask, with orders, attempts, validations and sessions under each, a detail panel for any row, and order chips to focus on one order; a built-in demo mode shows the view on example data without dispatching anything.
