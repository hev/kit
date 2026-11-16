---
id: "dispatch#inline-orders"
title: "Dispatch"
heading: "Inline orders"
group: "Use"
order: 94
url: "/docs/dispatch#inline-orders"
anchor: "inline-orders"
terms: ["inline","orders","subcommand","writes","order","small","tasks","taking","repo","task","optional","acceptance","text","reading","standard","input","dispatch","acme","typo","page","accept","says","found","printf","send","without","writing","file","repository","place","tell","name","host","picked","prints","docs","https","hevkit","pages","llms"]
hash: "84554be134c7aba21c615a2a16a9b7b244cc4c7e7ac031f1cd8c4917f3b69bc9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run acme/api 'Fix the typo in the 404 page' --accept 'The 404 page says \"not found\"'\nprintf 'Fix the typo in the 404 page\\n' | hev dispatch run acme/api -","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"Send a small task without writing an order file: run `hev dispatch run acme/api 'Fix the typo in the 404 page' --accept 'The 404 page says \"not found\"'`, using my repository in place of acme/api and my task and acceptance text. Tell me the run name and the host it picked, which it prints.\nDocs: https://hevkit.com/docs/dispatch#inline-orders (all pages: https://hevkit.com/llms.txt)","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"run","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"run REPO TASK...","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--host","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--name","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--base","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--harness","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--model","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--timeout","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--reply","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--issue","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--accept","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"validation","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"preview","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"default_host","chunkId":"dispatch#inline-orders"}]
sources: [{"chunkId":"dispatch#inline-orders","url":"/docs/dispatch#inline-orders","anchor":"inline-orders"}]
---

The run subcommand writes the order for small tasks, taking the repo, a task and optional acceptance text, or reading the task from standard input.
