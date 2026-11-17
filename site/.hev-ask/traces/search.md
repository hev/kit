---
id: "traces#search"
title: "Traces"
heading: "Search"
group: "Use"
order: 374
url: "/docs/traces#search"
anchor: "search"
terms: ["search","trace","terminal","type","remember","returns","matching","chunks","prompts","replies","tool","calls","their","sessions","eight","alternative","phrasings","fused","ranking","helps","pair","natural","language","question","exact","tokens","such","error","string","file","name","without","shared","archive","matches","words","only","says","miss","does"]
hash: "974f0a16361208093b8dedf8364185d0f638321913e669b4f1daa7ecfcb685d0"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces search \"why did the preflight fail\"","chunkId":"traces#search"},{"kind":"code","literal":"Search my coding-agent traces for QUESTION with `hev traces search \"QUESTION\"`. Tell me which sessions matched and quote the most relevant passage with its session ID. Say that this search matches words when no hev layer key is set, so a miss does not prove nothing exists.\nDocs: https://hevkit.com/docs/traces#search (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#search"},{"kind":"code","literal":"hev traces search \"why did the preflight fail\" --also \"HEVLAYER_API_KEY 401\"\nhev traces search --since 7d --harness codex --json \"flaky deploy\"","chunkId":"traces#search"},{"kind":"code","literal":"--also","chunkId":"traces#search"},{"kind":"code","literal":"--since","chunkId":"traces#search"},{"kind":"code","literal":"2h","chunkId":"traces#search"},{"kind":"code","literal":"7d","chunkId":"traces#search"},{"kind":"code","literal":"--harness","chunkId":"traces#search"},{"kind":"code","literal":"claude_code","chunkId":"traces#search"},{"kind":"code","literal":"codex","chunkId":"traces#search"},{"kind":"code","literal":"opencode","chunkId":"traces#search"},{"kind":"code","literal":"dot","chunkId":"traces#search"},{"kind":"code","literal":"pi","chunkId":"traces#search"},{"kind":"code","literal":"--workdir","chunkId":"traces#search"},{"kind":"code","literal":"--plan","chunkId":"traces#search"},{"kind":"code","literal":"--order","chunkId":"traces#search"},{"kind":"code","literal":"--context","chunkId":"traces#search"},{"kind":"code","literal":"--top","chunkId":"traces#search"},{"kind":"code","literal":"--json","chunkId":"traces#search"}]
sources: [{"chunkId":"traces#search","url":"/docs/traces#search","anchor":"search"}]
---

Trace search from the terminal: type what you remember and kit returns the matching chunks of prompts, replies and tool calls with their sessions. You can add up to eight alternative phrasings that are fused into one ranking, and it helps to pair a natural-language question with exact tokens such as an error string or file name. Without a shared archive it matches by words only and says so, so a miss does not mean nothing exists; a shared archive adds semantic matching. Results can be scoped by time window, harness, working directory, plan, order or context, and the scopes combine.
