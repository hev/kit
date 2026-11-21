---
id: "traces#what-gets-captured"
title: "Traces"
heading: "What gets captured"
group: "Use"
order: 438
url: "/docs/traces#what-gets-captured"
anchor: "what-gets-captured"
terms: ["gets","captured","hevd","scans","interval","default","five","minutes","config","harness","session","directory","claude","code","codex","opencode","value","scan","capture","toml","projects","sessions","agent","local","share","gemini","antigravity","brain","data","traces","transcript","full","jsonl","conversation","summaries","order","every","scaninterval","under","reads"]
hash: "e8cc6c0d990aaf15c9add3dc575c5ad742d4575d2d8e12ea9144430984a70c72"
mode: "agent-primary"
facts: [{"kind":"code","literal":"scan_interval","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"[capture]","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"--harness","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.claude/projects","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"claude_code","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.codex/sessions","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"codex","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.pi/agent/sessions","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"pi","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.local/share/opencode/opencode.db","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"opencode","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"agy","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.gemini/antigravity-cli/brain","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"HEV_AGY_DATA_DIR","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.dot/traces","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"dot","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"hev s","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"transcript_full.jsonl","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"transcript.jsonl","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"~/.gemini/antigravity-cli/brain/","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"conversation_summaries.db","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"agy_data_dir","chunkId":"traces#what-gets-captured"},{"kind":"code","literal":"--order","chunkId":"traces#what-gets-captured"}]
sources: [{"chunkId":"traces#what-gets-captured","url":"/docs/traces#what-gets-captured","anchor":"what-gets-captured"}]
---

hevd scans on an interval (default five minutes, set in config) over each harness's session directory: Claude Code, Codex, pi, OpenCode and agy, each with a harness value.
