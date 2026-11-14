---
id: "hosts#on-a-box-on-your-network"
title: "Hosts"
heading: "On a box on your network"
group: "Operate"
order: 173
url: "/docs/hosts#on-a-box-on-your-network"
anchor: "on-a-box-on-your-network"
terms: ["network","give","alias","config","connect","once","record","host","auth","working","verify","interactive","session","find","tmux","harness","adding","missing","directories","path","prefix","buildbox","hostname","example","test","user","batchmode","help","null","version","codex","name","laptop","check","finds","everything","headless","machine","keep","existing"]
hash: "c9f4eceb89ca1b3c9283216a8713a7cbef6ce13f30e6f6b939cbe0c6f802d4c6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Host buildbox\n  HostName buildbox.example.test\n  User dev","chunkId":"hosts#on-a-box-on-your-network"},{"kind":"code","literal":"ssh -o BatchMode=yes buildbox 'hev --help \u003e/dev/null \u0026\u0026 git --version \u0026\u0026 tmux -V \u0026\u0026 codex --version'","chunkId":"hosts#on-a-box-on-your-network"},{"kind":"code","literal":"~/.ssh/config","chunkId":"hosts#on-a-box-on-your-network"}]
sources: [{"chunkId":"hosts#on-a-box-on-your-network","url":"/docs/hosts#on-a-box-on-your-network","anchor":"on-a-box-on-your-network"}]
---

Give the box an alias in your SSH config, connect once to record its host key with key auth working, then verify a non-interactive SSH session can find hev, Git, tmux and the harness, adding missing directories to the host's path prefix.
