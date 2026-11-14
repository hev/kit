---
id: "dispatch#inline-orders"
title: "Dispatch"
heading: "Inline orders"
group: "Use"
order: 143
url: "/docs/dispatch#inline-orders"
anchor: "inline-orders"
terms: ["inline","orders","writes","order","repo","task","text","command","line","options","host","name","base","harness","model","timeout","reply","issue","acceptance","items","validation","preview","without","picks","default","free","else","reachable","most","slots","prints","choice","dispatch","acme","typo","page","accept","says","found","printf"]
hash: "84554be134c7aba21c615a2a16a9b7b244cc4c7e7ac031f1cd8c4917f3b69bc9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run acme/api 'Fix the typo in the 404 page' --accept 'The 404 page says \"not found\"'\nprintf 'Fix the typo in the 404 page\\n' | hev dispatch run acme/api -","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"Send a small task without writing an order file: run `hev dispatch run acme/api 'Fix the typo in the 404 page' --accept 'The 404 page says \"not found\"'`, using my repository in place of acme/api and my task and acceptance text. Tell me the run name and the host it picked, which it prints.\nDocs: https://hevkit.com/docs/dispatch#inline-orders (all pages: https://hevkit.com/llms.txt)","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"run","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"run REPO TASK...","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--host","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--name","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--base","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--harness","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--model","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--timeout","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--reply","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--issue","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"--accept","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"validation","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"preview","chunkId":"dispatch#inline-orders"},{"kind":"code","literal":"default_host","chunkId":"dispatch#inline-orders"}]
sources: [{"chunkId":"dispatch#inline-orders","url":"/docs/dispatch#inline-orders","anchor":"inline-orders"}]
---

An inline run writes the order for you from a repo and task text on the command line, with options for host, name, base, harness, model, timeout, reply, issue and acceptance items, but not validation or preview. Without a host it picks the default host if free, else the reachable host with the most free slots, and prints its choice.
