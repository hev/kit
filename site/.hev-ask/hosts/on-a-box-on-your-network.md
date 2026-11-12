---
id: "hosts#on-a-box-on-your-network"
title: "Hosts"
heading: "On a box on your network"
group: "Operate"
order: 118
url: "/docs/hosts#on-a-box-on-your-network"
anchor: "on-a-box-on-your-network"
terms: ["network","give","networked","alias","connect","once","record","host","authentication","check","interactive","session","finds","tmux","harness","adding","missing","directories","path","prefix","buildbox","hostname","example","test","user","batchmode","help","null","version","codex","config","name","laptop","everything","headless","machine","keep","existing","entries","true"]
hash: "c9f4eceb89ca1b3c9283216a8713a7cbef6ce13f30e6f6b939cbe0c6f802d4c6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Host buildbox\n  HostName buildbox.example.test\n  User dev","chunkId":"hosts#on-a-box-on-your-network"},{"kind":"code","literal":"ssh -o BatchMode=yes buildbox 'hev --help \u003e/dev/null \u0026\u0026 git --version \u0026\u0026 tmux -V \u0026\u0026 codex --version'","chunkId":"hosts#on-a-box-on-your-network"},{"kind":"code","literal":"~/.ssh/config","chunkId":"hosts#on-a-box-on-your-network"}]
sources: [{"chunkId":"hosts#on-a-box-on-your-network","url":"/docs/hosts#on-a-box-on-your-network","anchor":"on-a-box-on-your-network"}]
---

Give a networked box an SSH alias, connect once to record its host key with key authentication, then check that a non-interactive session finds hev, Git, tmux and the harness, adding missing directories to the host's PATH prefix.
