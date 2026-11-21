---
id: "clustering#the-config-file"
title: "Clustering"
heading: "The config file"
group: "Operate"
order: 130
url: "/docs/clustering#the-config-file"
anchor: "the-config-file"
terms: ["config","file","every","sending","receiving","machine","dispatch","table","known","node","cluster","names","describes","lookup","order","hosts","name","path","toml","sends","receives","work","knows","about","sender","reads","first","these","command","hevdispatchconfig","environment","variable","prefix","expands","home","directory","reading","same","format","senders"]
hash: "ecf1658c86b2b2667dedc83b270e1cc800e7615b171ff261bb0689646e47c890"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[hosts.NAME]","chunkId":"clustering#the-config-file"},{"kind":"code","literal":"--config PATH","chunkId":"clustering#the-config-file"},{"kind":"code","literal":"hev dispatch","chunkId":"clustering#the-config-file"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"clustering#the-config-file"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"clustering#the-config-file"},{"kind":"code","literal":"~/","chunkId":"clustering#the-config-file"}]
sources: [{"chunkId":"clustering#the-config-file","url":"/docs/clustering#the-config-file","anchor":"the-config-file"}]
---

Every sending or receiving machine has a dispatch config file with one table per known node; a node is in your cluster when your config names it. Describes the lookup order for the file.
