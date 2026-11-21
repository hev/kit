---
id: "clustering#invite-a-member"
title: "Clustering"
heading: "Invite a member"
group: "Operate"
order: 118
url: "/docs/clustering#invite-a-member"
anchor: "invite-a-member"
terms: ["invite","member","host","owner","makes","code","name","address","account","expiry","nodes","command","alex","build","example","test","user","expires","accept","request","allow","repos","also","remove","deny","join","administrators","programdata","authorized","keys","make","shared","private","network","workers","value","know","give","send","privately"]
hash: "85a14b672f5cdb383bc7d59ce2c8a1a2ee744d53523b2a67d84bb7b0e6fd99b4"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes invite alex --address build.example.test --user bot --expires 1h","chunkId":"clustering#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST\nhev nodes accept REQUEST --allow repos   # also let them add and remove repos\nhev nodes ls","chunkId":"clustering#invite-a-member"},{"kind":"code","literal":"hev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"clustering#invite-a-member"},{"kind":"code","literal":"hev nodes join CODE","chunkId":"clustering#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST --administrators","chunkId":"clustering#invite-a-member"},{"kind":"code","literal":"C:\\ProgramData\\ssh\\administrators_authorized_keys","chunkId":"clustering#invite-a-member"}]
sources: [{"chunkId":"clustering#invite-a-member","url":"/docs/clustering#invite-a-member","anchor":"invite-a-member"}]
---

On the host the owner makes a one-use invite code with member name, address, bot account and expiry via the nodes invite command.
