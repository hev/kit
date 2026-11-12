---
id: "share-a-host#invite-a-member"
title: "Shared hosts"
heading: "Invite a member"
group: "Operate"
order: 265
url: "/docs/share-a-host#invite-a-member"
anchor: "invite-a-member"
terms: ["invite","member","host","owner","creates","expiring","code","containing","address","pinned","token","sends","privately","joins","dedicated","alias","prints","request","verifies","accepts","optionally","granting","repos","permission","windows","administrator","account","extra","flag","writes","administrators","file","permissions","later","granted","revoked","without","nodes","alex","build"]
hash: "784fec6fd63c529d7538bb4b59d6a120e13e3eef6364f7137380989d913130bb"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes invite alex --address build.example.test --user bot --expires 1h","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST\nhev nodes accept REQUEST --allow repos   # also let them add and remove repos\nhev nodes ls","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes join CODE","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST --administrators","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"C:\\ProgramData\\ssh\\administrators_authorized_keys","chunkId":"share-a-host#invite-a-member"}]
sources: [{"chunkId":"share-a-host#invite-a-member","url":"/docs/share-a-host#invite-a-member","anchor":"invite-a-member"}]
---

The host owner creates a one-use, expiring invite code containing the host address, pinned host key and a token, and sends it privately. The member joins with the code, which creates a dedicated key and SSH alias and prints a request the owner verifies and accepts, optionally granting the repos permission; on Windows with an administrator SSH account an extra flag writes the administrators key file. Permissions can later be granted or revoked without a new invite.
