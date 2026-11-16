---
id: "peers#send-through-hevd"
title: "Peers"
heading: "Send through hevd"
group: "Use"
order: 227
url: "/docs/peers#send-through-hevd"
anchor: "send-through-hevd"
terms: ["send","through","hevd","default","peers","transport","receive","inbox","messages","section","destination","config","listen","address","random","secret","token","least","characters","keep","file","private","restart","capture","daemon","only","mode","messaging","needed","8789","replace","hosts","buildbox","https","example","test","toml","0600","hevconfig","replacewithrandomsecretatleast32characters"]
hash: "05c3a182535c5a15d4fc5bc6c4c16e2d96ce0ec75f4851d6e78bb9aaa422f4f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[peers]\nlisten = \"127.0.0.1:8789\"\ntoken = \"REPLACE_WITH_RANDOM_SECRET_AT_LEAST_32_CHARACTERS\"","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"[peers.hosts.buildbox]\nurl = \"https://peers.example.test\"\ntoken = \"REPLACE_WITH_DESTINATION_SECRET\"","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"HEV_CONFIG","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"0600","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"hev daemon --peers-only","chunkId":"peers#send-through-hevd"},{"kind":"code","literal":"via hevd","chunkId":"peers#send-through-hevd"}]
sources: [{"chunkId":"peers#send-through-hevd","url":"/docs/peers#send-through-hevd","anchor":"send-through-hevd"}]
---

SSH is the default peers transport. To receive inbox messages through hevd, add a peers section to the destination's config with a listen address and a random secret token of at least 32 characters, keep the file private, and restart capture or run the daemon in peers-only mode when only messaging is needed.
