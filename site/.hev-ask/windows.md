---
id: "windows"
title: "Windows"
heading: null
group: "Reference"
order: 413
url: "/docs/windows"
anchor: null
terms: ["windows","machine","capture","traces","take","dispatched","work","same","binary","macos","linux","because","tmux","launchd","daemon","runs","task","scheduler","supervisor","detached","process","steps","astro","codetabs","agentprompt","trace"]
hash: "520fabf6dcf478487ad70133d36ad1da1ef58940b8f76e165cbfba715a22e6b5"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"windows"},{"kind":"value","literal":"Steps.astro","chunkId":"windows"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"windows"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"windows"}]
sources: [{"chunkId":"windows","url":"/docs/windows","anchor":null}]
---

A Windows machine can capture its own traces and take dispatched work using the same binary as macOS and Linux. Because Windows has no tmux or launchd, the daemon runs from Task Scheduler and each run's supervisor is a detached process.
