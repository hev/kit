---
id: "quickstart#6-watch-it"
title: "Quickstart"
heading: "6. Watch it"
group: "Start"
order: 351
url: "/docs/quickstart#6-watch-it"
anchor: "6-watch-it"
terms: ["watch","quickstart","step","logs","attach","just","checking","instructions","through","worker","reads","between","steps","dispatch","first","check","tell","state","show","last","lines","without","command","ends","tmux","session","docs","https","hevkit","pages","llms","keep","section","under","send","running","instruction","confirm","message","orders"]
hash: "ceffbd48b30ffcfe16e169a7d55d8ef0584f9426799ac66f56c80ca459538076"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls\nhev dispatch logs first-run -f\nhev dispatch attach first-run","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"Check on the run first-run: run `hev dispatch ls` and tell me its state and age, then show the last 40 lines of `hev dispatch logs first-run` (without -f, so the command ends). Do not attach to its tmux session.\nDocs: https://hevkit.com/docs/quickstart#6-watch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"hev dispatch say first-run 'Keep the section under ten lines'","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"Send the running worker first-run a new instruction: run `hev dispatch say first-run 'Keep the section under ten lines'`. Confirm the message is in ~/.hev/orders/first-run/inbox.md.\nDocs: https://hevkit.com/docs/quickstart#6-watch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"hev dispatch show first-run","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"Read the receipt for the run first-run with `hev dispatch show first-run`. Summarize its status, the report and whether each Acceptance item was met.\nDocs: https://hevkit.com/docs/quickstart#6-watch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"ls","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"logs -f","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"attach","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"show","chunkId":"quickstart#6-watch-it"},{"kind":"code","literal":"~/.hev/orders/first-run/","chunkId":"quickstart#6-watch-it"}]
sources: [{"chunkId":"quickstart#6-watch-it","url":"/docs/quickstart#6-watch-it","anchor":"6-watch-it"}]
---

Quickstart step 6: watch with ls, logs -f and attach (don't attach if just checking); add instructions through say, which the worker reads between steps.
