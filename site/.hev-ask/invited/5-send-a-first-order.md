---
id: "invited#5-send-a-first-order"
title: "Your managed machine"
heading: "5. Send a first order"
group: "Start"
order: 263
url: "/docs/invited#5-send-a-first-order"
anchor: "5-send-a-first-order"
terms: ["send","first","order","step","harmless","read","only","command","node","host","repo","admits","confirm","accepted","queued","dispatch","readme","report","tests","change","files","push","open","name","xxxx","logs","show","please","also","list","test","commands","stop","notices","pull","once","watch","keep","checking","every"]
hash: "4bfc63d7181f9de32b84b3b196acaeee2ebb5b4e76dfb71da6b6db1f5b648c45"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run REPO \"Read README.md and report how to run the tests. Do not change files, push or open a PR.\" \\\n  --name first-run --host hev-node-XXXX","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev dispatch ls --host hev-node-XXXX\nhev dispatch logs first-run --host hev-node-XXXX\nhev dispatch show first-run --host hev-node-XXXX\nhev dispatch say first-run \"Please also list the test commands\" --host hev-node-XXXX\nhev dispatch stop first-run --host hev-node-XXXX","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev dispatch notices pull           # once\nhev dispatch notices pull --watch   # keep checking every minute","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"Pull the finished-run notices the shared host is holding for me: run `hev dispatch notices pull` once and tell me which of my runs finished, failed or stopped. Do not start the --watch form unless I ask.\nDocs: https://hevkit.com/docs/invited#5-send-a-first-order (all pages: https://hevkit.com/llms.txt)","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev-node-…","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev nodes ls","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"show","chunkId":"invited#5-send-a-first-order"}]
sources: [{"chunkId":"invited#5-send-a-first-order","url":"/docs/invited#5-send-a-first-order","anchor":"5-send-a-first-order"}]
---

Step 5: send a harmless read-only first order with the run command to the hev-node host, using a repo the host admits, and confirm it was accepted or queued.
