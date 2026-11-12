---
id: "traces#what-gets-captured"
title: "Traces"
heading: "What gets captured"
group: "Use"
order: 290
url: "/docs/traces#what-gets-captured"
anchor: "what-gets-captured"
terms: ["gets","captured","hevd","scans","every","five","minutes","default","configurable","reads","sessions","claude","code","codex","opencode","database","read","without","locking","supporting","versions","capturing","only","finished","replies","schema","changes","surfaced","status","errors","dispatched","orders","carry","order","filtering","scan","interval","capture","config","toml"]
hash: "1e5b634beee73082c51232d5c2e0f6902d18d163f384221fe168fde766aed00f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"scan_interval","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"[capture]","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"--harness","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.claude/projects","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"claude_code","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.codex/sessions","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"codex","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.pi/agent/sessions","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"pi","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.local/share/opencode/opencode.db","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"opencode","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.dot/traces","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"dot","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"hev s","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"--order","chunkId":"traces#what-gets-captured"},{"kind":"value","literal":"2.x","chunkId":"traces#what-gets-captured"},{"kind":"value","literal":"1.x","chunkId":"traces#what-gets-captured"}]
sources: [{"chunkId":"traces#what-gets-captured","url":"/docs/traces#what-gets-captured","anchor":"what-gets-captured"}]
---

hevd scans every five minutes by default (configurable) and reads sessions from Claude Code, Codex, pi, OpenCode and dot. OpenCode's database is read without locking, supporting versions 1 and 2 and capturing only finished replies, with schema changes surfaced as status errors. Sessions from dispatched orders carry the order ID for filtering.
