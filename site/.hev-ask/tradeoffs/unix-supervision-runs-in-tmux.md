---
id: "tradeoffs#unix-supervision-runs-in-tmux"
title: "Tradeoffs"
heading: "Unix supervision runs in tmux"
group: "Reference"
order: 330
url: "/docs/tradeoffs#unix-supervision-runs-in-tmux"
anchor: "unix-supervision-runs-in-tmux"
terms: ["unix","supervision","runs","tmux","macos","linux","supervisor","lives","session","attach","live","sessions","host","counts","slots","cost","hosts","need","forwards","fixed","list","environment","variables","because","keeps","starting","windows","uses","detached","processes","objects","name","named","watch","server","started","instead","inheriting"]
hash: "2949dbaa7a28ede01ad379d5d6e7a20ed81f2539af0360fccf224a4865f42186"
mode: "source-primary"
facts: [{"kind":"code","literal":"d-NAME","chunkId":"tradeoffs#unix-supervision-runs-in-tmux"}]
sources: [{"chunkId":"tradeoffs#unix-supervision-runs-in-tmux","url":"/docs/tradeoffs#unix-supervision-runs-in-tmux","anchor":"unix-supervision-runs-in-tmux"}]
---

On macOS and Linux each run's supervisor lives in a tmux session you can attach to, and live sessions are how a host counts its slots. The cost is that Unix hosts need tmux, and hev forwards a fixed list of environment variables because tmux keeps its starting environment. Windows uses detached processes and job objects with no attach.
