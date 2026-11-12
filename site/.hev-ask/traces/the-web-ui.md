---
id: "traces#the-web-ui"
title: "Traces"
heading: "The web UI"
group: "Use"
order: 285
url: "/docs/traces#the-web-ui"
anchor: "the-web-ui"
terms: ["serve","command","runs","local","trace","dashboard","localhost","port","8787","default","reading","file","archive","layer","bind","another","address","such","private","network","laptop","read","headless","machine","login","only","everyone","every","transcript","http","9000","namespace","traces","start","listens","taken","keep","verify","page","responds"]
hash: "7892de77b8109c726641dd8cb6a09c3ed67d429315f1bb220c413f58a9b02c06"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve                        # http://127.0.0.1:8787\nhev serve --port 9000 --namespace hev-traces","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"Start the hev trace dashboard on this machine with `hev serve`, which listens on http://127.0.0.1:8787 (use `--port PORT` if that port is taken). Keep it on 127.0.0.1. Verify the page responds with `curl -sI http://127.0.0.1:8787` and give me the URL.\nDocs: https://hevkit.com/docs/traces#the-web-ui (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"hev serve","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"127.0.0.1","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"--bind","chunkId":"traces#the-web-ui"}]
sources: [{"chunkId":"traces#the-web-ui","url":"/docs/traces#the-web-ui","anchor":"the-web-ui"}]
---

The serve command runs a local trace dashboard on localhost port 8787 by default, reading the file archive or the layer archive when a key is set. It can bind another address such as a private network IP so a laptop can read a headless machine's archive, but it has no login so only bind where everyone may read every transcript.
