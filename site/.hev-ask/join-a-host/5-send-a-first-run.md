---
id: "join-a-host#5-send-a-first-run"
title: "Join a host"
heading: "5. Send a first run"
group: "Operate"
order: 202
url: "/docs/join-a-host#5-send-a-first-run"
anchor: "5-send-a-first-run"
terms: ["send","first","harmless","read","only","admitted","repo","shared","host","list","logs","view","report","message","stop","busy","hosts","queue","because","cannot","reach","laptop","holds","notice","runs","finish","fail","pull","once","watch","loop","order","files","also","target","alias","dispatch","readme","tests","change"]
hash: "353a14b80498a22d5144b4062d4a83eac93e06e6b12ec66b6644ad9a3cde028f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch run REPO \"Read README.md and report how to run the tests. Do not change files, push or open a PR.\" \\\n  --name first-run --host hev-node-XXXX","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"hev dispatch ls --host hev-node-XXXX\nhev dispatch logs first-run --host hev-node-XXXX\nhev dispatch show first-run --host hev-node-XXXX\nhev dispatch say first-run \"Please also list the test commands\" --host hev-node-XXXX\nhev dispatch stop first-run --host hev-node-XXXX","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"hev dispatch notices pull           # once\nhev dispatch notices pull --watch   # keep checking every minute","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"Pull the finished-run notices the shared host is holding for me: run `hev dispatch notices pull` once and tell me which of my runs finished, failed or stopped. Do not start the --watch form unless I ask.\nDocs: https://hevkit.com/docs/join-a-host#5-send-a-first-run (all pages: https://hevkit.com/llms.txt)","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"show","chunkId":"join-a-host#5-send-a-first-run"},{"kind":"code","literal":"host:","chunkId":"join-a-host#5-send-a-first-run"}]
sources: [{"chunkId":"join-a-host#5-send-a-first-run","url":"/docs/join-a-host#5-send-a-first-run","anchor":"5-send-a-first-run"}]
---

Send a harmless read-only first run to an admitted repo on the shared host, then list, read logs, view the report, message or stop your own run. Busy hosts queue the run; because the host cannot reach your laptop, it holds a notice when your runs finish, fail or stop, which you pull once or on a watch loop. Order files can also target the host alias.
