---
id: "tradeoffs#unix-supervision-runs-in-tmux"
title: "Tradeoffs"
heading: "Unix supervision runs in tmux"
group: "Reference"
order: 297
url: "/docs/tradeoffs#unix-supervision-runs-in-tmux"
anchor: "unix-supervision-runs-in-tmux"
terms: ["unix","supervision","runs","tmux","macos","linux","supervisor","lives","named","session","attach","live","sessions","determine","slot","usage","hosts","need","forwards","fixed","list","environment","variables","windows","detached","processes","objects","name","watch","host","counts","slots","cost","server","keeps","started","instead","inheriting"]
hash: "2949dbaa7a28ede01ad379d5d6e7a20ed81f2539af0360fccf224a4865f42186"
mode: "source-primary"
facts: [{"kind":"code","literal":"d-NAME","chunkId":"tradeoffs#unix-supervision-runs-in-tmux"}]
sources: [{"chunkId":"tradeoffs#unix-supervision-runs-in-tmux","url":"/docs/tradeoffs#unix-supervision-runs-in-tmux","anchor":"unix-supervision-runs-in-tmux"}]
---

On macOS and Linux each run's supervisor lives in a named tmux session you can attach to, and live tmux sessions determine slot usage, so Unix hosts need tmux and hev forwards a fixed list of environment variables. Windows hosts use detached processes and job objects with no attach.
