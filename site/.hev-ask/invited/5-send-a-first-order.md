---
id: "invited#5-send-a-first-order"
title: "Your managed machine"
heading: "5. Send a first order"
group: "Start"
order: 193
url: "/docs/invited#5-send-a-first-order"
anchor: "5-send-a-first-order"
terms: ["send","first","order","start","read","only","against","repo","host","admits","machine","alias","follow","listing","reading","logs","showing","report","messaging","stopping","busy","queues","since","reach","laptop","pull","notices","finished","failed","stopped","runs","once","continuously","dispatch","readme","tests","change","files","push","open"]
hash: "4bfc63d7181f9de32b84b3b196acaeee2ebb5b4e76dfb71da6b6db1f5b648c45"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run REPO \"Read README.md and report how to run the tests. Do not change files, push or open a PR.\" \\\n  --name first-run --host hev-node-XXXX","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev dispatch ls --host hev-node-XXXX\nhev dispatch logs first-run --host hev-node-XXXX\nhev dispatch show first-run --host hev-node-XXXX\nhev dispatch say first-run \"Please also list the test commands\" --host hev-node-XXXX\nhev dispatch stop first-run --host hev-node-XXXX","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev dispatch notices pull           # once\nhev dispatch notices pull --watch   # keep checking every minute","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"Pull the finished-run notices the shared host is holding for me: run `hev dispatch notices pull` once and tell me which of my runs finished, failed or stopped. Do not start the --watch form unless I ask.\nDocs: https://hevkit.com/docs/invited#5-send-a-first-order (all pages: https://hevkit.com/llms.txt)","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev-node-…","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"hev nodes ls","chunkId":"invited#5-send-a-first-order"},{"kind":"code","literal":"show","chunkId":"invited#5-send-a-first-order"}]
sources: [{"chunkId":"invited#5-send-a-first-order","url":"/docs/invited#5-send-a-first-order","anchor":"5-send-a-first-order"}]
---

Start with a read-only first run against a repo the host admits, using the machine's alias, then follow it by listing, reading logs, showing the report, messaging or stopping it. A busy machine queues your order, and since the host can't reach your laptop you pull notices of finished, failed or stopped runs, once or continuously.
