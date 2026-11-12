---
id: "windows"
title: "Windows"
heading: null
group: "Reference"
order: 300
url: "/docs/windows"
anchor: null
terms: ["windows","machine","runs","same","binary","capture","traces","take","dispatched","work","tmux","launchd","daemon","task","scheduler","supervisor","detached","process","steps","astro","codetabs","agentprompt","trace","macos","linux"]
hash: "520fabf6dcf478487ad70133d36ad1da1ef58940b8f76e165cbfba715a22e6b5"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"windows"},{"kind":"value","literal":"Steps.astro","chunkId":"windows"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"windows"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"windows"}]
sources: [{"chunkId":"windows","url":"/docs/windows","anchor":null}]
---

A Windows machine runs the same hev binary to capture its own traces and take dispatched work. With no tmux or launchd, the daemon runs from Task Scheduler and each run's supervisor is a detached process.
