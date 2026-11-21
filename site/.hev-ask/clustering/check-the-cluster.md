---
id: "clustering#check-the-cluster"
title: "Clustering"
heading: "Check the cluster"
group: "Operate"
order: 115
url: "/docs/clustering#check-the-cluster"
anchor: "check-the-cluster"
terms: ["check","cluster","dispatch","hosts","command","reports","host","running","free","slots","reachability","version","flagging","mismatches","errors","give","whether","reachable","point","marked","mismatch","unreachable","error","name","about","docs","https","hevkit","clustering","pages","llms","json","update","default","channel","behind","buildbox","edge","1a2b3c4","commits"]
hash: "7f31b9418f865ee8b03baa6d8f2f909a160ec2c86ec11edd67c6cc89157e4ecf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch hosts","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"Check my dispatch hosts: run `hev dispatch hosts` and give me, for each host, its cap, running and free slots, whether it is reachable and its hev version. Point out any host marked (version mismatch) and any unreachable host with its SSH error. Use `--host NAME` if I ask about one host.\nDocs: https://hevkit.com/docs/clustering#check-the-cluster (all pages: https://hevkit.com/llms.txt)","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"hev","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"(version mismatch)","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"--json","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"--host NAME","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"hev dispatch hosts update --all","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"hev dispatch run","chunkId":"clustering#check-the-cluster"},{"kind":"code","literal":"default_host","chunkId":"clustering#check-the-cluster"}]
sources: [{"chunkId":"clustering#check-the-cluster","url":"/docs/clustering#check-the-cluster","anchor":"check-the-cluster"}]
---

The dispatch hosts command reports each host's cap, running and free slots, reachability and hev version, flagging version mismatches and SSH errors.
