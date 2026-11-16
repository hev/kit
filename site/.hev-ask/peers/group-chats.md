---
id: "peers#group-chats"
title: "Peers"
heading: "Group chats"
group: "Use"
order: 276
url: "/docs/peers#group-chats"
anchor: "group-chats"
terms: ["group","chats","several","peer","sessions","share","titled","conversation","created","listed","read","replied","chat","encodes","title","membership","remote","recipients","reply","keep","full","check","recipient","delivery","result","before","retrying","because","partly","fail","peers","reviewer","builder","review","change","show","passed","chatid","start","between"]
hash: "58e3efce78eb16be237247dfaa1c09e5b77e0615e536912f7e359873cc705421"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers chat new reviewer builder --title 'API review' -m 'Review this change'\nhev peers chat ls\nhev peers chat show CHAT_ID\nhev peers chat say CHAT_ID 'The check passed'","chunkId":"peers#group-chats"}]
sources: [{"chunkId":"peers#group-chats","url":"/docs/peers#group-chats","anchor":"group-chats"}]
---

Group chats let several peer sessions share a titled conversation that can be created, listed, read and replied to. The chat ID encodes title and membership so remote recipients can reply, so keep the full ID, and check each recipient's delivery result before retrying because delivery can partly fail.
