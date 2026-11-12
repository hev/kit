---
id: "peers#group-chats"
title: "Peers"
heading: "Group chats"
group: "Use"
order: 216
url: "/docs/peers#group-chats"
anchor: "group-chats"
terms: ["group","chats","start","chat","between","several","peer","sessions","title","first","message","list","show","post","encodes","membership","remote","recipients","reply","normal","transport","keep","full","check","recipient","delivery","result","before","retrying","because","partly","fail","peers","reviewer","builder","review","change","passed","chatid","peer1"]
hash: "58e3efce78eb16be237247dfaa1c09e5b77e0615e536912f7e359873cc705421"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers chat new reviewer builder --title 'API review' -m 'Review this change'\nhev peers chat ls\nhev peers chat show CHAT_ID\nhev peers chat say CHAT_ID 'The check passed'","chunkId":"peers#group-chats"}]
sources: [{"chunkId":"peers#group-chats","url":"/docs/peers#group-chats","anchor":"group-chats"}]
---

You can start a group chat between several peer sessions with a title and first message, list chats, show one and post to it. The chat ID encodes its title and membership so remote recipients can reply over normal peer transport, so keep the full ID, and check each recipient's delivery result before retrying because delivery can partly fail.
