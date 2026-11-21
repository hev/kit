---
id: "clustering#on-a-box-on-your-network"
title: "Clustering"
heading: "On a box on your network"
group: "Operate"
order: 120
url: "/docs/clustering#on-a-box-on-your-network"
anchor: "on-a-box-on-your-network"
terms: ["network","give","alias","laptop","check","interactive","session","finds","harness","host","buildbox","hostname","example","test","user","batchmode","help","null","version","tmux","codex","config","name","everything","headless","machine","keep","existing","entries","connect","once","true","lands","knownhosts","make","sure","authentication","works","without","password"]
hash: "19ed9d831f41394e7abdb7f7cb0a61b13221fe422352fd86cd4b0ced4b3cdde8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Host buildbox\n  HostName buildbox.example.test\n  User dev","chunkId":"clustering#on-a-box-on-your-network"},{"kind":"code","literal":"ssh -o BatchMode=yes buildbox 'hev --help \u003e/dev/null \u0026\u0026 git --version \u0026\u0026 tmux -V \u0026\u0026 codex --version'","chunkId":"clustering#on-a-box-on-your-network"},{"kind":"code","literal":"~/.ssh/config","chunkId":"clustering#on-a-box-on-your-network"}]
sources: [{"chunkId":"clustering#on-a-box-on-your-network","url":"/docs/clustering#on-a-box-on-your-network","anchor":"on-a-box-on-your-network"}]
---

Give the box an SSH alias on your laptop and check that a non-interactive session finds hev and the harness.
