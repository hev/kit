---
id: "dispatch#workspaces"
title: "Dispatch"
heading: "Workspaces"
group: "Use"
order: 203
url: "/docs/dispatch#workspaces"
anchor: "workspaces"
terms: ["workspaces","works","checkout","branch","name","plain","worktree","host","clone","created","origin","base","removed","clean","sharing","nothing","between","runs","dispatch","train","released","need","license","save","time","disk","parallel","without","worktrees","workspace","mode","auto","features","licensed","require","root","driver","apfs","linux","reflink"]
hash: "4bce7c12710ceeb10b1e1ea56df3319ed23ecbe0085450cb386a35cefcb4e073"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap = 8\n\n[workspace]\nmode = \"auto\"            # auto: Pro features when licensed; oss: plain Git worktrees; pro: require a license\nroot = \"~/workspace\"\ndriver = \"cow\"           # cow: APFS clone or Linux reflink (XFS, Btrfs), falling back to Git; git: always Git checkout\nauto_compact_done = false","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"\u003crun\u003e/wt","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"git worktree","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"clean","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"cow","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"driver = \"git\"","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"cap","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"Cargo.toml","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"cargo-target","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"CARGO_TARGET_DIR","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"target/","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"hev dispatch compact NAME","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"--all","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"origin","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"rework","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"retry","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"amend","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"close","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"auto_compact_done = true","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"hev dispatch gc","chunkId":"dispatch#workspaces"},{"kind":"code","literal":"workspace = \"~/workspace\"","chunkId":"dispatch#workspaces"},{"kind":"value","literal":"0.7","chunkId":"dispatch#workspaces"}]
sources: [{"chunkId":"dispatch#workspaces","url":"/docs/dispatch#workspaces","anchor":"workspaces"}]
---

Each run works in its own checkout on branch d/NAME; in OSS this is a plain git worktree of the host's clone created from origin/base and removed by clean, sharing nothing between runs. Pro dispatch workspaces (0.7 train, not yet released) need a Kit Pro license on the host to save time and disk for parallel runs; without one plain worktrees are used.
