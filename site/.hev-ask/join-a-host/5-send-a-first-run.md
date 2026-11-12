---
id: "join-a-host#5-send-a-first-run"
title: "Join a host"
heading: "5. Send a first run"
group: "Operate"
order: 147
url: "/docs/join-a-host#5-send-a-first-run"
anchor: "5-send-a-first-run"
terms: ["send","first","read","only","admitted","repo","host","alias","follow","list","logs","show","message","stop","busy","hosts","queue","runs","finished","notices","pulled","order","files","target","their","dispatch","readme","report","tests","change","push","open","name","node","xxxx","please","also","test","commands","pull"]
hash: "353a14b80498a22d5144b4062d4a83eac93e06e6b12ec66b6644ad9a3cde028f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run REPO \"Read README.md and report how to run the tests. Do not change files, push or open a PR.\" \\\n  --name first-run --host hev-node-XXXX","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"hev dispatch ls --host hev-node-XXXX\nhev dispatch logs first-run --host hev-node-XXXX\nhev dispatch show first-run --host hev-node-XXXX\nhev dispatch say first-run \"Please also list the test commands\" --host hev-node-XXXX\nhev dispatch stop first-run --host hev-node-XXXX","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"hev dispatch notices pull           # once\nhev dispatch notices pull --watch   # keep checking every minute","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"Pull the finished-run notices the shared host is holding for me: run `hev dispatch notices pull` once and tell me which of my runs finished, failed or stopped. Do not start the --watch form unless I ask.\nDocs: https://hevkit.com/docs/join-a-host#5-send-a-first-run (all pages: https://hevkit.com/llms.txt)","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"show","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"host:","chunkId":"join-a-host#5-send-a-first-run"}]
sources: [{"chunkId":"join-a-host#5-send-a-first-run","url":"/docs/join-a-host#5-send-a-first-run","anchor":"5-send-a-first-run"}]
---

Send a read-only first run to an admitted repo on the host alias and follow it with list, logs, show, message and stop. Busy hosts queue runs, finished-run notices are pulled from the host, and order files can target the alias as their host.
