---
id: "peers#send-a-message"
title: "Peers"
heading: "Send a message"
group: "Use"
order: 220
url: "/docs/peers#send-a-message"
anchor: "send-a-message"
terms: ["send","message","peer","short","saying","about","need","pipe","longer","standard","input","interactive","recipient","without","inbox","pasted","prompt","header","naming","sender","reply","receiver","knows","came","never","tmux","keystrokes","check","later","reading","session","peers","rebase","main","repo","hold","push","echo","preflight","board"]
hash: "c8f2ed7205e5865bed715eef9fca1ded83eb7efe9da0c6d8e232606709048ba7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers send SESSION \"I'm about to rebase main in this repo. Hold your push.\"\necho \"The preflight fix is on the board, thread 7.\" | hev peers send SESSION -","chunkId":"peers#send-a-message"}]
sources: [{"chunkId":"peers#send-a-message","url":"/docs/peers#send-a-message","anchor":"send-a-message"}]
---

Send a peer a short message saying what you are about to do and what you need, or pipe a longer message from standard input. For an interactive recipient without an inbox the message is pasted as a prompt with a header naming the sender and how to reply, so the receiver knows it came from a peer; never use raw tmux keystrokes, and check later for a reply by reading the session.
