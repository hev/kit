---
id: "share-a-host#invite-a-member"
title: "Shared hosts"
heading: "Invite a member"
group: "Operate"
order: 335
url: "/docs/share-a-host#invite-a-member"
anchor: "invite-a-member"
terms: ["invite","member","owner","creates","expiring","code","host","containing","address","pinned","token","sends","privately","after","joins","returns","request","verifies","came","accepts","optionally","granting","repo","permission","extra","step","windows","administrator","accounts","permissions","changed","later","without","nodes","alex","build","example","test","user","expires"]
hash: "784fec6fd63c529d7538bb4b59d6a120e13e3eef6364f7137380989d913130bb"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes invite alex --address build.example.test --user bot --expires 1h","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST\nhev nodes accept REQUEST --allow repos   # also let them add and remove repos\nhev nodes ls","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes join CODE","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST --administrators","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"C:\\ProgramData\\ssh\\administrators_authorized_keys","chunkId":"share-a-host#invite-a-member"}]
sources: [{"chunkId":"share-a-host#invite-a-member","url":"/docs/share-a-host#invite-a-member","anchor":"invite-a-member"}]
---

The owner creates a one-use, expiring invite code on the host containing the address, pinned host key and a token, and sends it privately. After the member joins and returns a request, the owner verifies it came from them and accepts it, optionally granting repo permission (with an extra step for Windows administrator accounts); permissions can be changed later without a new invite.
