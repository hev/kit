---
id: "quickstart#6-watch-it"
title: "Quickstart"
heading: "6. Watch it"
group: "Start"
order: 234
url: "/docs/quickstart#6-watch-it"
anchor: "6-watch-it"
terms: ["watch","list","running","runs","state","trace","follow","worker","attach","tmux","session","instructions","appended","inbox","file","reads","between","steps","finishes","receipt","shows","metadata","status","notes","report","result","also","disk","directory","dispatch","logs","first","check","tell","show","last","lines","without","command","ends"]
hash: "6c8e726f69408c90c5b08bb3b7a0cf616c3e84c1e6f429c0110bd9dd32911ad8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls\nhev dispatch logs first-run -f\nhev dispatch attach first-run","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"Check on the run first-run: run `hev dispatch ls` and tell me its state and age, then show the last 40 lines of `hev dispatch logs first-run` (without -f, so the command ends). Do not attach to its tmux session.\nDocs: https://hevkit.com/docs/quickstart#6-watch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"hev dispatch say first-run 'Keep the section under ten lines'","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"Send the running worker first-run a new instruction: run `hev dispatch say first-run 'Keep the section under ten lines'`. Confirm the message is in ~/dispatch/first-run/inbox.md.\nDocs: https://hevkit.com/docs/quickstart#6-watch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"hev dispatch show first-run","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"Read the receipt for the run first-run with `hev dispatch show first-run`. Summarize its status, the report and whether each Acceptance item was met.\nDocs: https://hevkit.com/docs/quickstart#6-watch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"ls","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"logs -f","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"attach","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"show","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"~/dispatch/first-run/","chunkId":"quickstart#6-watch-it"}]
sources: [{"chunkId":"quickstart#6-watch-it","url":"/docs/quickstart#6-watch-it","anchor":"6-watch-it"}]
---

You can list running runs with state, age and trace, follow the worker's log, or attach to its tmux session. Mid-run instructions are appended to the run's inbox file, which the worker reads between steps, and when it finishes the receipt shows metadata, status notes, report, result and inbox, all also on disk in the run directory.
