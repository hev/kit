---
id: "traces#search"
title: "Traces"
heading: "Search"
group: "Use"
order: 336
url: "/docs/traces#search"
anchor: "search"
terms: ["search","prompts","replies","tool","calls","across","sessions","adding","alternate","phrasings","eight","fused","ranking","pairing","natural","language","literal","tokens","like","error","strings","alone","matches","words","bm25","shared","archive","phrasing","also","gets","semantic","matching","filters","cover","time","window","harness","working","directory","plan"]
hash: "055b3ec309c9f8bc3a8a631795bb32c6349491098afd16901c9a93eafd448b6c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces search \"why did the preflight fail\"","chunkId":"traces#search"},{"kind":"code","literal":"Search my coding-agent traces for QUESTION with `hev traces search \"QUESTION\"`. Tell me which sessions matched and quote the most relevant passage with its session ID. Say that this search matches words when no hev layer key is set, so a miss does not prove nothing exists.\nDocs: https://hevkit.com/docs/traces#search (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#search"},{"kind":"code","literal":"hev traces search \"why did the preflight fail\" --also \"HEVLAYER_API_KEY 401\"\nhev traces search --since 7d --harness codex --json \"flaky deploy\"","chunkId":"traces#search"},{"kind":"code","literal":"--also","chunkId":"traces#search"},{"kind":"code","literal":"--since","chunkId":"traces#search"},{"kind":"code","literal":"2h","chunkId":"traces#search"},{"kind":"code","literal":"7d","chunkId":"traces#search"},{"kind":"code","literal":"--harness","chunkId":"traces#search"},{"kind":"code","literal":"claude_code","chunkId":"traces#search"},{"kind":"code","literal":"codex","chunkId":"traces#search"},{"kind":"code","literal":"opencode","chunkId":"traces#search"},{"kind":"code","literal":"dot","chunkId":"traces#search"},{"kind":"code","literal":"pi","chunkId":"traces#search"},{"kind":"code","literal":"--workdir","chunkId":"traces#search"},{"kind":"code","literal":"--plan","chunkId":"traces#search"},{"kind":"code","literal":"--order","chunkId":"traces#search"},{"kind":"code","literal":"--context","chunkId":"traces#search"},{"kind":"code","literal":"--top","chunkId":"traces#search"},{"kind":"code","literal":"--json","chunkId":"traces#search"}]
sources: [{"chunkId":"traces#search","url":"/docs/traces#search","anchor":"search"}]
---

Search prompts, replies and tool calls across all sessions, adding alternate phrasings (up to eight, fused into one ranking) and pairing natural language with literal tokens like error strings. Alone, kit matches words (BM25); with a shared archive each phrasing also gets semantic matching. Filters cover time window, harness, working directory, plan, order, context digest, result count and JSON output, combined with AND.
