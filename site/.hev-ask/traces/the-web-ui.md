---
id: "traces#the-web-ui"
title: "Traces"
heading: "The web UI"
group: "Use"
order: 379
url: "/docs/traces#the-web-ui"
anchor: "the-web-ui"
terms: ["local","dashboard","server","reads","file","archive","layer","present","listens","only","machine","default","bound","another","address","example","laptop","read","headless","login","bind","everyone","reach","every","transcript","serve","http","8787","port","9000","namespace","traces","start","trace","taken","keep","verify","page","responds","curl"]
hash: "7892de77b8109c726641dd8cb6a09c3ed67d429315f1bb220c413f58a9b02c06"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve                        # http://127.0.0.1:8787\nhev serve --port 9000 --namespace hev-traces","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"Start the hev trace dashboard on this machine with `hev serve`, which listens on http://127.0.0.1:8787 (use `--port PORT` if that port is taken). Keep it on 127.0.0.1. Verify the page responds with `curl -sI http://127.0.0.1:8787` and give me the URL.\nDocs: https://hevkit.com/docs/traces#the-web-ui (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"hev serve","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"127.0.0.1","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"--bind","chunkId":"traces#the-web-ui"}]
sources: [{"chunkId":"traces#the-web-ui","url":"/docs/traces#the-web-ui","anchor":"the-web-ui"}]
---

A local dashboard server reads the file archive, or the hev layer archive when a key is present. It listens only on the local machine by default and can be bound to another address, for example so a laptop can read a headless machine's archive. The server has no login, so bind it only where everyone who can reach it may read every transcript.
