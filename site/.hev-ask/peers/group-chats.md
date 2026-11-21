---
id: "peers#group-chats"
title: "Peers"
heading: "Group chats"
group: "Use"
order: 339
url: "/docs/peers#group-chats"
anchor: "group-chats"
terms: ["group","chats","several","peer","sessions","share","titled","chat","create","command","list","show","keep","full","check","recipient","delivery","result","since","partly","fail","peers","reviewer","builder","title","review","change","passed","chatid","start","between","peer1","peer2","first","message","prints","before","retrying","anything","confirm"]
hash: "58e3efce78eb16be237247dfaa1c09e5b77e0615e536912f7e359873cc705421"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers chat new reviewer builder --title 'API review' -m 'Review this change'\nhev peers chat ls\nhev peers chat show CHAT_ID\nhev peers chat say CHAT_ID 'The check passed'","chunkId":"peers#group-chats"}]
sources: [{"chunkId":"peers#group-chats","url":"/docs/peers#group-chats","anchor":"group-chats"}]
---

Group chats let several peer sessions share a titled chat: create with the chat new command, list, show and say. Keep the full chat ID and check each recipient's delivery result since delivery can partly fail.
