---
id: "join-a-host#when-something-goes-wrong"
title: "Join a host"
heading: "When something goes wrong"
group: "Operate"
order: 205
url: "/docs/join-a-host#when-something-goes-wrong"
anchor: "when-something-goes-wrong"
terms: ["something","goes","wrong","troubleshooting","table","joining","unreachable","host","means","network","invite","accepted","expired","invitations","need","code","publickey","permission","errors","mean","owner","revoked","access","were","removed","refused","runs","repo","admitted","leave","remove","delete","node","files","config","block","dispatch","entry","invitation","already"]
hash: "bc781570a5532ee6c00d11b22d1fa0d9d01284f78394f0e962281f28adf7dbfa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"invitation expired","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"host already joined","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"hev nodes ls","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"Permission denied (publickey)","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"access revoked or unknown","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"hev-node-…","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"~/.ssh/","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"~/.ssh/config","chunkId":"join-a-host#when-something-goes-wrong"},{"kind":"code","literal":"dispatch.toml","chunkId":"join-a-host#when-something-goes-wrong"}]
sources: [{"chunkId":"join-a-host#when-something-goes-wrong","url":"/docs/join-a-host#when-something-goes-wrong","anchor":"when-something-goes-wrong"}]
---

Troubleshooting table for joining: unreachable host means the network invite was not accepted, expired invitations need a new code, publickey permission errors mean the owner has not accepted yet, revoked access means you were removed, and refused runs mean the repo is not admitted. To leave, ask the owner to remove you, then delete the node's key files, SSH config block and dispatch config entry.
