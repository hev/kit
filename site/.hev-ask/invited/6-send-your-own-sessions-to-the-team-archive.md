---
id: "invited#6-send-your-own-sessions-to-the-team-archive"
title: "Your managed machine"
heading: "6. Send your own sessions to the team archive"
group: "Start"
order: 194
url: "/docs/invited#6-send-your-own-sessions-to-the-team-archive"
anchor: "6-send-your-own-sessions-to-the-team-archive"
terms: ["send","sessions","team","archive","runs","machine","already","trace","laptop","setup","namespace","adam","1password","link","only","environment","plus","installing","daemon","windows","check","status","capture","includes","every","agent","session","secret","scrubbing","perfect","work","export","hevlayer","install","recorded","there","everyone","search","planned","reviewed"]
hash: "4795eb92b5313dd377af628fd3754f53f0c40de36b2da725947079a61f2155a6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"export HEVLAYER_API_KEY=KEY\nexport HEVLAYER_NAMESPACE=NAMESPACE\nhev up\nhev s","chunkId":"invited#6-send-your-own-sessions-to-the-team-archive"},{"kind":"code","literal":"$env:HEVLAYER_API_KEY = \"KEY\"\n$env:HEVLAYER_NAMESPACE = \"NAMESPACE\"\nhev.exe up\nhev.exe daemon install\nhev.exe s","chunkId":"invited#6-send-your-own-sessions-to-the-team-archive"},{"kind":"code","literal":"hev up","chunkId":"invited#6-send-your-own-sessions-to-the-team-archive"},{"kind":"code","literal":"hev s","chunkId":"invited#6-send-your-own-sessions-to-the-team-archive"}]
sources: [{"chunkId":"invited#6-send-your-own-sessions-to-the-team-archive","url":"/docs/invited#6-send-your-own-sessions-to-the-team-archive","anchor":"6-send-your-own-sessions-to-the-team-archive"}]
---

Runs on the machine already go to the team trace archive; to add your laptop's own sessions, run setup with the team API key and namespace from Adam's 1Password link set only in the environment (plus installing the daemon on Windows), then check the daemon status. Capture includes every agent session on the laptop, and secret scrubbing is not perfect, so use a work machine.
