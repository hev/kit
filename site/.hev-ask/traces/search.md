---
id: "traces#search"
title: "Traces"
heading: "Search"
group: "Use"
order: 429
url: "/docs/traces#search"
anchor: "search"
terms: ["search","traces","question","includes","ready","made","prompt","asking","agent","without","matches","words","only","semantic","grading","needs","layer","preflight","fail","coding","tell","sessions","matched","quote","most","relevant","passage","session","miss","does","prove","nothing","exists","docs","https","hevkit","pages","llms","also","hevlayer"]
hash: "2f815d14db7453419af2b78adca0b5ecd02562b09b2d81a62ebd85f69cb6fec6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces search \"why did the preflight fail\"","chunkId":"traces#search"},{"kind":"code","literal":"Search my coding-agent traces for QUESTION with `hev traces search \"QUESTION\"`. Tell me which sessions matched and quote the most relevant passage with its session ID. Say that this search matches words when no hev layer key is set, so a miss does not prove nothing exists.\nDocs: https://hevkit.com/docs/traces#search (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#search"},{"kind":"code","literal":"hev traces search \"why did the preflight fail\" --also \"HEVLAYER_API_KEY 401\"\nhev traces search --since 7d --harness codex --json \"flaky deploy\"","chunkId":"traces#search"},{"kind":"code","literal":"--also","chunkId":"traces#search"},{"kind":"code","literal":"--since","chunkId":"traces#search"},{"kind":"code","literal":"2h","chunkId":"traces#search"},{"kind":"code","literal":"7d","chunkId":"traces#search"},{"kind":"code","literal":"--harness","chunkId":"traces#search"},{"kind":"code","literal":"claude_code","chunkId":"traces#search"},{"kind":"code","literal":"codex","chunkId":"traces#search"},{"kind":"code","literal":"opencode","chunkId":"traces#search"},{"kind":"code","literal":"agy","chunkId":"traces#search"},{"kind":"code","literal":"dot","chunkId":"traces#search"},{"kind":"code","literal":"pi","chunkId":"traces#search"},{"kind":"code","literal":"--workdir","chunkId":"traces#search"},{"kind":"code","literal":"--plan","chunkId":"traces#search"},{"kind":"code","literal":"--order","chunkId":"traces#search"},{"kind":"code","literal":"--context","chunkId":"traces#search"},{"kind":"code","literal":"--top","chunkId":"traces#search"},{"kind":"code","literal":"--json","chunkId":"traces#search"}]
sources: [{"chunkId":"traces#search","url":"/docs/traces#search","anchor":"search"}]
---

Search traces from the CLI with a question; includes a ready-made prompt for asking an agent to search. Without a key it matches words only, and semantic grading needs a hev layer key.
