---
id: "clustering#5-send-a-first-run"
title: "Clustering"
heading: "5. Send a first run"
group: "Operate"
order: 109
url: "/docs/clustering#5-send-a-first-run"
anchor: "5-send-a-first-run"
terms: ["send","first","step","read","only","repo","host","admits","naming","verify","setup","without","changes","dispatch","readme","report","tests","change","files","push","open","name","node","xxxx","logs","show","please","also","list","test","commands","stop","notices","pull","once","watch","keep","checking","every","minute"]
hash: "f1bae16485332c115706512867823ab7d0f464b5fa9477d5539fa9fe93d719c8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run REPO \"Read README.md and report how to run the tests. Do not change files, push or open a PR.\" \\\n  --name first-run --host hev-node-XXXX","chunkId":"clustering#5-send-a-first-run"},{"kind":"code","literal":"hev dispatch ls --host hev-node-XXXX\nhev dispatch logs first-run --host hev-node-XXXX\nhev dispatch show first-run --host hev-node-XXXX\nhev dispatch say first-run \"Please also list the test commands\" --host hev-node-XXXX\nhev dispatch stop first-run --host hev-node-XXXX","chunkId":"clustering#5-send-a-first-run"},{"kind":"code","literal":"hev dispatch notices pull           # once\nhev dispatch notices pull --watch   # keep checking every minute","chunkId":"clustering#5-send-a-first-run"},{"kind":"code","literal":"Pull the finished-run notices the shared host is holding for me: run `hev dispatch notices pull` once and tell me which of my runs finished, failed or stopped. Do not start the --watch form unless I ask.\nDocs: https://hevkit.com/docs/clustering#5-send-a-first-run (all pages: https://hevkit.com/llms.txt)","chunkId":"clustering#5-send-a-first-run"},{"kind":"code","literal":"show","chunkId":"clustering#5-send-a-first-run"},{"kind":"code","literal":"host:","chunkId":"clustering#5-send-a-first-run"}]
sources: [{"chunkId":"clustering#5-send-a-first-run","url":"/docs/clustering#5-send-a-first-run","anchor":"5-send-a-first-run"}]
---

Step 5: send a read-only first run on a repo the host admits, naming the run and host, to verify the setup without changes.
