---
id: "share-a-host#invite-a-member"
title: "Shared hosts"
heading: "Invite a member"
group: "Operate"
order: 321
url: "/docs/share-a-host#invite-a-member"
anchor: "invite-a-member"
terms: ["invite","member","owner","creates","expiring","code","containing","host","address","pinned","fingerprint","token","sends","privately","after","joins","verifies","accepts","their","request","optionally","granting","repos","permission","windows","administrators","option","account","admin","later","grant","revoke","permissions","without","nodes","alex","build","example","test","user"]
hash: "784fec6fd63c529d7538bb4b59d6a120e13e3eef6364f7137380989d913130bb"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes invite alex --address build.example.test --user bot --expires 1h","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST\nhev nodes accept REQUEST --allow repos   # also let them add and remove repos\nhev nodes ls","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes join CODE","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"hev nodes accept REQUEST --administrators","chunkId":"share-a-host#invite-a-member"},{"kind":"code","literal":"C:\\ProgramData\\ssh\\administrators_authorized_keys","chunkId":"share-a-host#invite-a-member"}]
sources: [{"chunkId":"share-a-host#invite-a-member","url":"/docs/share-a-host#invite-a-member","anchor":"invite-a-member"}]
---

The owner creates a one-use, expiring invite code containing the host's address, pinned key, fingerprint and a token, and sends it privately. After the member joins, the owner verifies and accepts their request, optionally granting the repos permission (and on Windows, using the administrators option if the SSH account is an admin), and can later grant or revoke permissions without a new invite.
