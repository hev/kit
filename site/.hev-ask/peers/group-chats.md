---
id: "peers#group-chats"
title: "Peers"
heading: "Group chats"
group: "Use"
order: 222
url: "/docs/peers#group-chats"
anchor: "group-chats"
terms: ["group","chats","among","peer","sessions","created","chat","listed","shown","posted","keep","full","carries","title","membership","remote","recipients","reply","check","recipient","delivery","result","before","retrying","because","partly","fail","peers","reviewer","builder","review","change","show","passed","chatid","start","between","peer1","peer2","first"]
hash: "58e3efce78eb16be237247dfaa1c09e5b77e0615e536912f7e359873cc705421"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers chat new reviewer builder --title 'API review' -m 'Review this change'\nhev peers chat ls\nhev peers chat show CHAT_ID\nhev peers chat say CHAT_ID 'The check passed'","chunkId":"peers#group-chats"}]
sources: [{"chunkId":"peers#group-chats","url":"/docs/peers#group-chats","anchor":"group-chats"}]
---

Group chats among peer sessions are created with chat new, listed, shown and posted to with chat say. Keep the full chat ID, which carries title and membership so remote recipients can reply, and check each recipient's delivery result before retrying because delivery can partly fail.
