---
id: "tradeoffs#unix-supervision-runs-in-tmux"
title: "Tradeoffs"
heading: "Unix supervision runs in tmux"
group: "Reference"
order: 353
url: "/docs/tradeoffs#unix-supervision-runs-in-tmux"
anchor: "unix-supervision-runs-in-tmux"
terms: ["unix","supervision","runs","tmux","macos","linux","supervisor","lives","named","session","attach","live","sessions","count","slots","costs","required","only","fixed","list","environment","variables","forwarded","windows","uses","detached","processes","objects","name","watch","host","counts","cost","hosts","need","server","keeps","started","forwards","instead"]
hash: "2949dbaa7a28ede01ad379d5d6e7a20ed81f2539af0360fccf224a4865f42186"
mode: "source-primary"
facts: [{"kind":"code","literal":"d-NAME","chunkId":"tradeoffs#unix-supervision-runs-in-tmux"}]
sources: [{"chunkId":"tradeoffs#unix-supervision-runs-in-tmux","url":"/docs/tradeoffs#unix-supervision-runs-in-tmux","anchor":"unix-supervision-runs-in-tmux"}]
---

On macOS and Linux each run's supervisor lives in a named tmux session you can attach to, and live sessions count slots. Costs: tmux is required and only a fixed list of environment variables is forwarded; Windows uses detached processes and job objects with no attach.
