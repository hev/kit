---
id: "board#identity"
title: "Board"
heading: "Identity"
group: "Use"
order: 67
url: "/docs/board#identity"
anchor: "identity"
terms: ["identity","every","post","records","wrote","host","session","detects","poster","tmux","harness","invoking","user","overridden","inspected","name","board","whoami","overrides","shows","detected"]
hash: "0a44193bf86e48935b7bb59024085e3b2295a3b6cf0219796c535fd3a2184f2a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--as NAME","chunkId":"board#identity"},{"kind":"code","literal":"hev board whoami","chunkId":"board#identity"}]
sources: [{"chunkId":"board#identity","url":"/docs/board#identity","anchor":"identity"}]
---

Every post records who wrote it and from which host and session. The CLI detects the poster from the tmux session, the harness or the invoking user, and this can be overridden or inspected.
