---
id: "share-a-host#invite-a-member"
title: "Shared hosts"
heading: "Invite a member"
group: "Operate"
order: 277
url: "/docs/share-a-host#invite-a-member"
anchor: "invite-a-member"
terms: ["invite","member","owner","makes","code","nodes","giving","name","host","private","network","address","account","expiry","holds","pinned","fingerprint","send","privately","never","post","joins","join","alex","build","example","test","user","expires","accept","request","allow","repos","also","remove","deny","administrators","programdata","authorized","keys"]
hash: "784fec6fd63c529d7538bb4b59d6a120e13e3eef6364f7137380989d913130bb"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes invite alex --address build.example.test --user bot --expires 1h","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST\nhev nodes accept REQUEST --allow repos   # also let them add and remove repos\nhev nodes ls","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes join CODE","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST --administrators","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"C:\\ProgramData\\ssh\\administrators_authorized_keys","chunkId":"share-a-host#invite-a-member"}]
sources: [{"chunkId":"share-a-host#invite-a-member","url":"/docs/share-a-host#invite-a-member","anchor":"invite-a-member"}]
---

The owner makes a one-use invite code with hev nodes invite, giving the member's name, the host's private-network address, the host account and an expiry. The code holds the address, pinned host key and fingerprint, and expiry; send it privately and never post it. The member then joins with hev nodes join.
