---
id: "hosts#checking-hosts"
title: "Hosts"
heading: "Checking hosts"
group: "Operate"
order: 177
url: "/docs/hosts#checking-hosts"
anchor: "checking-hosts"
terms: ["checking","hosts","host","inventory","asks","every","parallel","shows","capacity","running","free","slots","reachability","version","update","channel","whether","newer","build","published","flagging","mismatches","giving","unreachable","line","error","inline","dispatch","uses","same","pick","preferring","default","reachable","otherwise","most","queueing","full","check","give"]
hash: "f5117d2f232f360028d521dc0c99a346667523596b6a0391070f9ae9b17ed156"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch hosts","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"Check my dispatch hosts: run `hev dispatch hosts` and give me, for each host, its cap, running and free slots, whether it is reachable and its hev version. Point out any host marked (version mismatch) and any unreachable host with its SSH error. Use `--host NAME` if I ask about one host.\nDocs: https://hevkit.com/docs/hosts#checking-hosts (all pages: https://hevkit.com/llms.txt)","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"hev","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"(version mismatch)","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"--json","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"--host NAME","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"hev dispatch hosts update --all","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"hev dispatch run","chunkId":"hosts#checking-hosts"},{"kind":"code","literal":"default_host","chunkId":"hosts#checking-hosts"}]
sources: [{"chunkId":"hosts#checking-hosts","url":"/docs/hosts#checking-hosts","anchor":"checking-hosts"}]
---

The host inventory asks every host in parallel and shows capacity, running and free slots, reachability, version, update channel and whether a newer build is published, flagging version mismatches and giving unreachable hosts one line with the SSH error. Inline dispatch uses the same inventory to pick a host, preferring the default host when reachable and free, otherwise the one with the most free slots, and queueing when all are full.
