---
id: "hosts#checking-hosts"
title: "Hosts"
heading: "Checking hosts"
group: "Operate"
order: 170
url: "/docs/hosts#checking-hosts"
anchor: "checking-hosts"
terms: ["checking","hosts","queries","every","host","parallel","timeout","shows","capacity","running","free","slots","reachability","version","flagging","mismatches","unreachable","inline","runs","same","inventory","pick","preferring","default","otherwise","most","queuing","full","dispatch","check","give","whether","reachable","point","marked","mismatch","error","name","about","docs"]
hash: "d9f850b2659e143b42c73ad6013fd84030c51101e09d1a0c5ceb937c0ea72bf6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch hosts","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"Check my dispatch hosts: run `hev dispatch hosts` and give me, for each host, its cap, running and free slots, whether it is reachable and its hev version. Point out any host marked (version mismatch) and any unreachable host with its SSH error. Use `--host NAME` if I ask about one host.\nDocs: https://hevkit.com/docs/hosts#checking-hosts (all pages: https://hevkit.com/llms.txt)","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"HOST      CAP  RUNNING  FREE  REACHABLE  VERSION\nbuildbox  8    3        5     yes        dev\nhere      2    0        2     yes        dev\nmac: unreachable: ssh: exit status 255: ssh: connect to host mac port 22: Operation timed out; for mutations the outcome is unknown: inspect the named run before retrying","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"hev","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"(version mismatch)","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"--json","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"--host NAME","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"hev dispatch run","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"default_host","chunkId":"hosts#checking-hosts"}]
sources: [{"chunkId":"hosts#checking-hosts","url":"/docs/hosts#checking-hosts","anchor":"checking-hosts"}]
---

Checking hosts queries every host in parallel with a timeout and shows capacity, running and free slots, reachability and hev version, flagging version mismatches and unreachable hosts. Inline runs use the same inventory to pick a host, preferring the default host and otherwise the one with most free slots, queuing when all are full.
