---
id: "peers#send-a-message"
title: "Peers"
heading: "Send a message"
group: "Use"
order: 343
url: "/docs/peers#send-a-message"
anchor: "send-a-message"
terms: ["send","message","peers","delivers","short","piped","longer","text","session","about","need","never","tmux","keys","check","replies","show","interactive","recipients","without","inbox","pasted","their","terminal","rebase","main","repo","hold","push","echo","preflight","board","thread","peer","pipe","confirm","printed","error","reply","later"]
hash: "c8f2ed7205e5865bed715eef9fca1ded83eb7efe9da0c6d8e232606709048ba7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev peers send SESSION \"I'm about to rebase main in this repo. Hold your push.\"\necho \"The preflight fix is on the board, thread 7.\" | hev peers send SESSION -","chunkId":"peers#send-a-message"}]
sources: [{"chunkId":"peers#send-a-message","url":"/docs/peers#send-a-message","anchor":"send-a-message"}]
---

hev peers send delivers a short message (or piped longer text) to a session; say what you're about to do and what you need, never use raw tmux send-keys, and check for replies with show. Interactive recipients without an inbox get the message pasted into their terminal.
