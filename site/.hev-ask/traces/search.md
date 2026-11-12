---
id: "traces#search"
title: "Traces"
heading: "Search"
group: "Use"
order: 281
url: "/docs/traces#search"
anchor: "search"
terms: ["search","traces","finds","prompts","replies","tool","calls","across","sessions","returns","matching","chunks","their","session","without","layer","matches","words","only","miss","does","prove","nothing","exists","pass","eight","alternative","phrasings","also","option","pairing","natural","language","literal","tokens","like","error","strings","fused","ranking"]
hash: "62438d847b880b3a04e40ee3e30306c2a59c52299882c2ac789d2d6b01c68bf0"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces search \"why did the preflight fail\"","chunkId":"traces#search"},{"kind":"code","literal":"Search my coding-agent traces for QUESTION with `hev traces search \"QUESTION\"`. Tell me which sessions matched and quote the most relevant passage with its session ID. Say that this search matches words when no hev layer key is set, so a miss does not prove nothing exists.\nDocs: https://hevkit.com/docs/traces#search (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#search"},{"kind":"code","literal":"hev traces search \"why did the preflight fail\" --also \"HEVLAYER_API_KEY 401\"\nhev traces search --since 7d --harness codex --json \"flaky deploy\"","chunkId":"traces#search"},{"kind":"code","literal":"--also","chunkId":"traces#search"},{"kind":"code","literal":"--since","chunkId":"traces#search"},{"kind":"code","literal":"2h","chunkId":"traces#search"},{"kind":"code","literal":"7d","chunkId":"traces#search"},{"kind":"code","literal":"--harness","chunkId":"traces#search"},{"kind":"code","literal":"claude_code","chunkId":"traces#search"},{"kind":"code","literal":"codex","chunkId":"traces#search"},{"kind":"code","literal":"opencode","chunkId":"traces#search"},{"kind":"code","literal":"dot","chunkId":"traces#search"},{"kind":"code","literal":"pi","chunkId":"traces#search"},{"kind":"code","literal":"--workdir","chunkId":"traces#search"},{"kind":"code","literal":"--plan","chunkId":"traces#search"},{"kind":"code","literal":"--order","chunkId":"traces#search"},{"kind":"code","literal":"--context","chunkId":"traces#search"},{"kind":"code","literal":"--top","chunkId":"traces#search"},{"kind":"code","literal":"--json","chunkId":"traces#search"}]
sources: [{"chunkId":"traces#search","url":"/docs/traces#search","anchor":"search"}]
---

hev traces search finds prompts, replies and tool calls across all sessions and returns matching chunks with their session. Without a layer key it matches words only, so a miss does not prove nothing exists. Pass up to eight alternative phrasings with the also option, pairing natural language with literal tokens like error strings, and they are fused into one ranking.
