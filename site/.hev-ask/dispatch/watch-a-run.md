---
id: "dispatch#watch-a-run"
title: "Dispatch"
heading: "Watch a run"
group: "Use"
order: 160
url: "/docs/dispatch#watch-a-run"
anchor: "watch-a-run"
terms: ["watch","follow","runs","listing","checks","every","host","parallel","default","shows","running","died","need","review","validation","activity","option","include","finished","details","print","metadata","status","report","result","inbox","files","plus","trace","hint","logs","followed","until","ends","attaching","joins","tmux","session","inventory","capacity"]
hash: "5268320280336d6c870b15ef53d961efae1b2641df71567d5742866998cdb84b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls\nhev dispatch ls --all --json\nhev dispatch show api-rate-limit-headers\nhev dispatch logs api-rate-limit-headers -f\nhev dispatch attach api-rate-limit-headers\nhev dispatch hosts","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"ls","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"--all","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"show NAME","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"meta.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"status.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"report.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"result.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"validation.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"verdict.json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"--json","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"order.md","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"logs NAME","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"-f","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"attach NAME","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"logs -f","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"hosts","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"hev","chunkId":"dispatch#watch-a-run"},{"kind":"code","literal":"--host","chunkId":"dispatch#watch-a-run"}]
sources: [{"chunkId":"dispatch#watch-a-run","url":"/docs/dispatch#watch-a-run","anchor":"watch-a-run"}]
---

How to follow runs: the listing checks every host in parallel and by default shows runs that are running, died, need review or have validation activity, with an option to include finished runs. Run details print the metadata, status, report, result, inbox and validation files plus a trace hint; logs can be followed until the run ends; attaching joins the run's tmux session; and the host inventory shows capacity, reachability and version mismatches. Name-based commands search every host and need a host specified if the name exists on more than one.
