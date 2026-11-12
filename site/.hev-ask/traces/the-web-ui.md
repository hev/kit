---
id: "traces#the-web-ui"
title: "Traces"
heading: "The web UI"
group: "Use"
order: 286
url: "/docs/traces#the-web-ui"
anchor: "the-web-ui"
terms: ["serve","runs","dashboard","localhost","default","reading","file","archive","layer","port","option","bind","allows","another","address","such","private","network","read","headless","machine","there","login","only","everyone","reaching","every","transcript","http","8787","9000","namespace","traces","start","trace","listens","taken","keep","verify","page"]
hash: "7892de77b8109c726641dd8cb6a09c3ed67d429315f1bb220c413f58a9b02c06"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve                        # http://127.0.0.1:8787\nhev serve --port 9000 --namespace hev-traces","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"Start the hev trace dashboard on this machine with `hev serve`, which listens on http://127.0.0.1:8787 (use `--port PORT` if that port is taken). Keep it on 127.0.0.1. Verify the page responds with `curl -sI http://127.0.0.1:8787` and give me the URL.\nDocs: https://hevkit.com/docs/traces#the-web-ui (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"hev serve","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"127.0.0.1","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"--bind","chunkId":"traces#the-web-ui"}]
sources: [{"chunkId":"traces#the-web-ui","url":"/docs/traces#the-web-ui","anchor":"the-web-ui"}]
---

hev serve runs the dashboard on localhost by default, reading the file archive or your layer archive, with a port option. A bind option allows another address, such as a private network address to read a headless machine's archive. There is no login, so bind only where everyone reaching it may read every transcript.
