---
id: "traces#what-gets-captured"
title: "Traces"
heading: "What gets captured"
group: "Use"
order: 384
url: "/docs/traces#what-gets-captured"
anchor: "what-gets-captured"
terms: ["gets","captured","daemon","scans","configurable","interval","five","minutes","default","reading","session","sources","claude","code","codex","opencode","database","read","without","locking","changing","while","runs","across","versions","capturing","replies","only","once","finished","surfacing","error","status","layout","changes","sessions","part","dispatched","order","carry"]
hash: "1e5b634beee73082c51232d5c2e0f6902d18d163f384221fe168fde766aed00f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"scan_interval","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"[capture]","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"--harness","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.claude/projects","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"claude_code","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.codex/sessions","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"codex","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.pi/agent/sessions","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"pi","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.local/share/opencode/opencode.db","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"opencode","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.dot/traces","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"dot","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"hev s","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"--order","chunkId":"traces#what-gets-captured"},{"kind":"value","literal":"2.x","chunkId":"traces#what-gets-captured"},{"kind":"value","literal":"1.x","chunkId":"traces#what-gets-captured"}]
sources: [{"chunkId":"traces#what-gets-captured","url":"/docs/traces#what-gets-captured","anchor":"what-gets-captured"}]
---

The daemon scans on a configurable interval, five minutes by default, reading session sources for Claude Code, Codex, pi, OpenCode and dot. OpenCode's session database is read without locking or changing it while OpenCode runs, across its 1.x and 2.x versions, capturing replies only once finished and surfacing an error in status if the layout changes. Sessions run as part of a dispatched order carry the order's ID, so all traces behind one piece of work can be pulled up together.
