---
id: "peers#send-a-message"
title: "Peers"
heading: "Send a message"
group: "Use"
order: 226
url: "/docs/peers#send-a-message"
anchor: "send-a-message"
terms: ["send","message","peers","delivers","short","session","reads","longer","standard","input","interactive","recipient","without","inbox","gets","pasted","prompt","header","naming","sender","reply","route","knows","came","peer","check","replies","later","show","about","rebase","main","repo","hold","push","echo","preflight","board","thread","pipe"]
hash: "c8f2ed7205e5865bed715eef9fca1ded83eb7efe9da0c6d8e232606709048ba7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers send SESSION \"I'm about to rebase main in this repo. Hold your push.\"\necho \"The preflight fix is on the board, thread 7.\" | hev peers send SESSION -","chunkId":"peers#send-a-message"}]
sources: [{"chunkId":"peers#send-a-message","url":"/docs/peers#send-a-message","anchor":"send-a-message"}]
---

hev peers send delivers a short message to a session, or reads a longer one from standard input. An interactive recipient without an inbox gets it pasted as a prompt with a header naming the sender and reply route, so it knows it came from a peer. Check for replies later with show.
