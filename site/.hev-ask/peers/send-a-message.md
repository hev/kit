---
id: "peers#send-a-message"
title: "Peers"
heading: "Send a message"
group: "Use"
order: 270
url: "/docs/peers#send-a-message"
anchor: "send-a-message"
terms: ["send","message","short","peer","session","pipe","longer","standard","input","without","inbox","pasted","prompt","header","naming","sender","reply","recipient","knows","came","rather","human","peers","about","rebase","main","repo","hold","push","echo","preflight","board","thread","need","never","tmux","keys","confirm","printed","error"]
hash: "c8f2ed7205e5865bed715eef9fca1ded83eb7efe9da0c6d8e232606709048ba7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers send SESSION \"I'm about to rebase main in this repo. Hold your push.\"\necho \"The preflight fix is on the board, thread 7.\" | hev peers send SESSION -","chunkId":"peers#send-a-message"}]
sources: [{"chunkId":"peers#send-a-message","url":"/docs/peers#send-a-message","anchor":"send-a-message"}]
---

Send a short message to a peer session, or pipe a longer one from standard input. Without an inbox, it is pasted as a prompt with a header naming the sender and how to reply, so the recipient knows it came from a peer rather than the human.
