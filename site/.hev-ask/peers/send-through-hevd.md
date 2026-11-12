---
id: "peers#send-through-hevd"
title: "Peers"
heading: "Send through hevd"
group: "Use"
order: 221
url: "/docs/peers#send-through-hevd"
anchor: "send-through-hevd"
terms: ["send","through","hevd","default","peer","transport","machine","receive","inbox","messages","http","daemon","adding","peers","listener","address","long","random","token","config","file","mode","0600","senders","configure","destination","host","delivery","only","enqueues","registered","inboxes","connection","failures","before","submission","fall","back","while","auth"]
hash: "05c3a182535c5a15d4fc5bc6c4c16e2d96ce0ec75f4851d6e78bb9aaa422f4f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[peers]\nlisten = \"127.0.0.1:8789\"\ntoken = \"REPLACE_WITH_RANDOM_SECRET_AT_LEAST_32_CHARACTERS\"","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"[peers.hosts.buildbox]\nurl = \"https://peers.example.test\"\ntoken = \"REPLACE_WITH_DESTINATION_SECRET\"","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"HEV_CONFIG","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"0600","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"hev daemon --peers-only","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"via hevd","chunkId":"peers#send-through-hevd"}]
sources: [{"chunkId":"peers#send-through-hevd","url":"/docs/peers#send-through-hevd","anchor":"send-through-hevd"}]
---

SSH is the default peer transport, but a machine can receive inbox messages over HTTP through the hevd daemon by adding a peers listener address and a long random token to its hev config (file mode 0600), and senders configure each destination host's URL and token. HTTP delivery only enqueues to registered inboxes; connection failures before submission fall back to SSH, while auth errors or timeouts do not because delivery may have happened. This lets outbound-only hosts such as Windows behind NAT send messages without inbound SSH, though inventory and reads still need SSH.
