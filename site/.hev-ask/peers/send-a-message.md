---
id: "peers#send-a-message"
title: "Peers"
heading: "Send a message"
group: "Use"
order: 280
url: "/docs/peers#send-a-message"
anchor: "send-a-message"
terms: ["send","message","sending","peer","interactive","session","without","inbox","pastes","prompt","header","naming","sender","reply","recipient","knows","came","operator","longer","messages","piped","standard","input","peers","about","rebase","main","repo","hold","push","echo","preflight","board","thread","short","pipe","need","never","tmux","keys"]
hash: "c8f2ed7205e5865bed715eef9fca1ded83eb7efe9da0c6d8e232606709048ba7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers send SESSION \"I'm about to rebase main in this repo. Hold your push.\"\necho \"The preflight fix is on the board, thread 7.\" | hev peers send SESSION -","chunkId":"peers#send-a-message"}]
sources: [{"chunkId":"peers#send-a-message","url":"/docs/peers#send-a-message","anchor":"send-a-message"}]
---

Sending a peer message to an interactive session without an inbox pastes it as a prompt with a header naming the sender and how to reply, so the recipient knows it came from a peer and not the operator. Longer messages can be piped in from standard input.
