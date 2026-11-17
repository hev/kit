---
id: "dispatch#watch-a-run"
title: "Dispatch"
heading: "Watch a run"
group: "Use"
order: 104
url: "/docs/dispatch#watch-a-run"
anchor: "watch-a-run"
terms: ["watch","runs","show","logs","attach","hosts","subcommands","optional","json","output","following","page","includes","prompt","report","name","host","state","validation","dispatch","rate","limit","headers","meta","status","result","inbox","verdict","order","list","replace","summarize","last","lines","without","command","ends","also","flag","unreachable"]
hash: "5268320280336d6c870b15ef53d961efae1b2641df71567d5742866998cdb84b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls\nhev dispatch ls --all --json\nhev dispatch show api-rate-limit-headers\nhev dispatch logs api-rate-limit-headers -f\nhev dispatch attach api-rate-limit-headers\nhev dispatch hosts","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"ls","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"--all","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"show NAME","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"meta.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"status.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"report.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"result.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"validation.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"verdict.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"--json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"order.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"logs NAME","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"-f","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"attach NAME","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"logs -f","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"hosts","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"hev","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"--host","chunkId":"dispatch#watch-a-run"}]
sources: [{"chunkId":"dispatch#watch-a-run","url":"/docs/dispatch#watch-a-run","anchor":"watch-a-run"}]
---

Watch runs with the ls, show, logs, attach and hosts subcommands, with optional JSON output and following logs. The page includes a prompt to report each run's name, host, state, validation, age and PR.
