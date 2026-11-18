---
id: "hosts#on-a-box-on-your-network"
title: "Hosts"
heading: "On a box on your network"
group: "Operate"
order: 180
url: "/docs/hosts#on-a-box-on-your-network"
anchor: "on-a-box-on-your-network"
terms: ["network","give","alias","config","connect","once","host","known","authentication","works","without","prompt","check","interactive","session","finds","tmux","harness","anything","found","there","needs","directory","added","path","prefix","buildbox","hostname","example","test","user","batchmode","help","null","version","codex","name","laptop","everything","headless"]
hash: "c9f4eceb89ca1b3c9283216a8713a7cbef6ce13f30e6f6b939cbe0c6f802d4c6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Host buildbox\n  HostName buildbox.example.test\n  User dev","chunkId":"hosts#on-a-box-on-your-network"},{"kind":"code","literal":"ssh -o BatchMode=yes buildbox 'hev --help \u003e/dev/null \u0026\u0026 git --version \u0026\u0026 tmux -V \u0026\u0026 codex --version'","chunkId":"hosts#on-a-box-on-your-network"},{"kind":"code","literal":"~/.ssh/config","chunkId":"hosts#on-a-box-on-your-network"}]
sources: [{"chunkId":"hosts#on-a-box-on-your-network","url":"/docs/hosts#on-a-box-on-your-network","anchor":"on-a-box-on-your-network"}]
---

For a box on your network: give it an alias in your SSH config, connect once so its host key is known and key authentication works without a prompt, then check that a non-interactive SSH session finds hev, Git, tmux and the harness. Anything not found there needs its directory added to that host's PATH prefix.
