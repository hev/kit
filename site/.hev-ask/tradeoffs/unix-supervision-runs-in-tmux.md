---
id: "tradeoffs#unix-supervision-runs-in-tmux"
title: "Tradeoffs"
heading: "Unix supervision runs in tmux"
group: "Reference"
order: 392
url: "/docs/tradeoffs#unix-supervision-runs-in-tmux"
anchor: "unix-supervision-runs-in-tmux"
terms: ["unix","supervision","runs","tmux","macos","linux","supervisor","lives","session","attach","live","sessions","host","counts","slots","cost","hosts","need","because","server","keeps","starting","environment","forwards","fixed","list","variables","rather","inheriting","windows","detached","processes","objects","instead","name","named","watch","started"]
hash: "2949dbaa7a28ede01ad379d5d6e7a20ed81f2539af0360fccf224a4865f42186"
mode: "source-primary"
facts: [{"kind":"code","literal":"d-NAME","chunkId":"tradeoffs#unix-supervision-runs-in-tmux"}]
sources: [{"chunkId":"tradeoffs#unix-supervision-runs-in-tmux","url":"/docs/tradeoffs#unix-supervision-runs-in-tmux","anchor":"unix-supervision-runs-in-tmux"}]
---

On macOS and Linux each run's supervisor lives in its own tmux session that you can attach to, and live sessions are how a host counts its slots. The cost is that Unix hosts need tmux and, because a tmux server keeps its starting environment, hev forwards a fixed list of environment variables to each run rather than inheriting them. Windows hosts use detached processes and job objects instead, with no attach.
