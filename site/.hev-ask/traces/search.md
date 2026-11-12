---
id: "traces#search"
title: "Traces"
heading: "Search"
group: "Use"
order: 280
url: "/docs/traces#search"
anchor: "search"
terms: ["search","trace","takes","natural","language","question","returns","matching","chunks","prompts","replies","tool","calls","their","sessions","eight","alternative","phrasings","fused","ranking","ideally","pairing","literal","error","strings","flags","file","names","scope","time","window","harness","working","directory","plan","order","context","json","output","agents"]
hash: "2e36f990d8d7791de1bd97a99493e4f8d190063e35221437a56c2136c76d1006"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces search \"why did the preflight fail\"","chunkId":"traces#search"},{"kind":"code","literal":"Search my coding-agent traces for QUESTION with `hev traces search \"QUESTION\"`. Tell me which sessions matched and quote the most relevant passage with its session ID. Say that this search matches words when no hev layer key is set, so a miss does not prove nothing exists.\nDocs: https://hevkit.com/docs/traces#search (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#search"},{"kind":"code","literal":"hev traces search \"why did the preflight fail\" --also \"HEVLAYER_API_KEY 401\"\nhev traces search --since 7d --harness codex --json \"flaky deploy\"","chunkId":"traces#search"},{"kind":"code","literal":"--also","chunkId":"traces#search"},{"kind":"code","literal":"--since","chunkId":"traces#search"},{"kind":"code","literal":"2h","chunkId":"traces#search"},{"kind":"code","literal":"7d","chunkId":"traces#search"},{"kind":"code","literal":"--harness","chunkId":"traces#search"},{"kind":"code","literal":"claude_code","chunkId":"traces#search"},{"kind":"code","literal":"codex","chunkId":"traces#search"},{"kind":"code","literal":"opencode","chunkId":"traces#search"},{"kind":"code","literal":"dot","chunkId":"traces#search"},{"kind":"code","literal":"pi","chunkId":"traces#search"},{"kind":"code","literal":"--workdir","chunkId":"traces#search"},{"kind":"code","literal":"--plan","chunkId":"traces#search"},{"kind":"code","literal":"--order","chunkId":"traces#search"},{"kind":"code","literal":"--context","chunkId":"traces#search"},{"kind":"code","literal":"--top","chunkId":"traces#search"},{"kind":"code","literal":"--json","chunkId":"traces#search"}]
sources: [{"chunkId":"traces#search","url":"/docs/traces#search","anchor":"search"}]
---

Trace search takes a natural-language question and returns matching chunks of prompts, replies and tool calls with their sessions. You can add up to eight alternative phrasings fused into one ranking, ideally pairing a natural question with literal error strings, flags or file names, and scope by time window, harness, working directory, plan, order or context with JSON output for agents. Alone kit uses word matching (BM25); with kit plus layer each phrasing also gets semantic matching.
