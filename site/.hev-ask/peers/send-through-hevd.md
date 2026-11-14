---
id: "peers#send-through-hevd"
title: "Peers"
heading: "Send through hevd"
group: "Use"
order: 271
url: "/docs/peers#send-through-hevd"
anchor: "send-through-hevd"
terms: ["send","through","hevd","default","peer","transport","machine","receive","inbox","messages","http","daemon","configuring","loopback","listener","strong","token","senders","configure","destination","only","enqueues","connection","failures","fall","back","other","errors","there","retry","queue","outbound","hosts","such","windows","behind","still","peers","listen","8789"]
hash: "05c3a182535c5a15d4fc5bc6c4c16e2d96ce0ec75f4851d6e78bb9aaa422f4f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[peers]\nlisten = \"127.0.0.1:8789\"\ntoken = \"REPLACE_WITH_RANDOM_SECRET_AT_LEAST_32_CHARACTERS\"","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"[peers.hosts.buildbox]\nurl = \"https://peers.example.test\"\ntoken = \"REPLACE_WITH_DESTINATION_SECRET\"","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"HEV_CONFIG","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"0600","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"hev daemon --peers-only","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"via hevd","chunkId":"peers#send-through-hevd"}]
sources: [{"chunkId":"peers#send-through-hevd","url":"/docs/peers#send-through-hevd","anchor":"send-through-hevd"}]
---

SSH is the default peer transport, but a machine can receive inbox messages over HTTP through the daemon by configuring a loopback listener and a strong token, and senders configure each destination's URL and token. HTTP only enqueues; connection failures fall back to SSH but other errors do not, there is no retry queue, and outbound-only hosts such as Windows behind NAT can still send.
