---
id: "traces#the-web-ui"
title: "Traces"
heading: "The web UI"
group: "Use"
order: 433
url: "/docs/traces#the-web-ui"
anchor: "the-web-ui"
terms: ["serve","runs","dashboard","traces","list","stats","transcripts","timelines","read","only","board","reading","file","archive","layer","http","8787","port","9000","namespace","start","trace","machine","listens","taken","keep","verify","page","responds","curl","give","docs","https","hevkit","pages","llms","8099","bind","tabs","session"]
hash: "b666f2bf86b6933fc171793cf10dd5016c49da8be6a8dd87825b300db2b0279a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve                        # http://127.0.0.1:8787\nhev serve --port 9000 --namespace hev-traces","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"Start the hev trace dashboard on this machine with `hev serve`, which listens on http://127.0.0.1:8787 (use `--port PORT` if that port is taken). Keep it on 127.0.0.1. Verify the page responds with `curl -sI http://127.0.0.1:8787` and give me the URL.\nDocs: https://hevkit.com/docs/traces#the-web-ui (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"hev serve","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"hev up","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"http://127.0.0.1:8099","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"127.0.0.1:8787","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"--port","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"127.0.0.1","chunkId":"traces#the-web-ui"},{"kind":"code","literal":"--bind","chunkId":"traces#the-web-ui"}]
sources: [{"chunkId":"traces#the-web-ui","url":"/docs/traces#the-web-ui","anchor":"the-web-ui"}]
---

hev serve runs the dashboard with a Traces tab (list/stats, transcripts, timelines) and a read-only Board, reading the file archive or the hev layer archive.
