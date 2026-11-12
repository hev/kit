---
id: "quickstart#5-dispatch-it"
title: "Quickstart"
heading: "5. Dispatch it"
group: "Start"
order: 233
url: "/docs/quickstart#5-dispatch-it"
anchor: "5-dispatch-it"
terms: ["dispatch","dispatching","order","fetches","origin","creates","branch","main","worktree","under","directory","starts","supervisor","tmux","session","owns","timeout","writes","result","agent","exits","first","confirm","created","refuses","show","exact","error","docs","https","hevkit","quickstart","pages","llms"]
hash: "7117549e78fedb8e7982dea8077a8e432851944af2e8cbf21403ffa1d2378bcd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch first-run.md","chunkId":"quickstart#5-dispatch-it"},{"kind":"code","literal":"Dispatch the order: run `hev dispatch first-run.md`. Confirm it created branch d/first-run, a worktree at ~/dispatch/first-run/wt and a tmux session d-first-run. If dispatch refuses, show me the exact error.\nDocs: https://hevkit.com/docs/quickstart#5-dispatch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#5-dispatch-it"},{"kind":"code","literal":"origin","chunkId":"quickstart#5-dispatch-it"},{"kind":"code","literal":"d/first-run","chunkId":"quickstart#5-dispatch-it"},{"kind":"code","literal":"origin/main","chunkId":"quickstart#5-dispatch-it"},{"kind":"code","literal":"~/dispatch/first-run/wt","chunkId":"quickstart#5-dispatch-it"},{"kind":"code","literal":"d-first-run","chunkId":"quickstart#5-dispatch-it"}]
sources: [{"chunkId":"quickstart#5-dispatch-it","url":"/docs/quickstart#5-dispatch-it","anchor":"5-dispatch-it"}]
---

Dispatching the order fetches origin, creates a d/ branch from main in a new worktree under the dispatch directory, and starts a supervisor in a tmux session. The supervisor owns the timeout and writes the result when the agent exits.
