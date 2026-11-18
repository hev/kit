---
id: "hosts#reaching-a-host"
title: "Hosts"
heading: "Reaching a host"
group: "Operate"
order: 182
url: "/docs/hosts#reaching-a-host"
anchor: "reaching-a-host"
terms: ["reaching","host","either","local","running","work","calling","machine","reached","through","destination","such","user","pair","config","alias","runs","batch","mode","short","connect","timeout","strict","checking","never","prompts","authentication","first","remote","side","must","find","interactive","path","prefix","setting","handles","tools","installed","only"]
hash: "a82a8f8be6214677e3367cedf64d70b607ed1c49df535ef17d205c7964a07000"
mode: "agent-primary"
facts: [{"kind":"code","literal":"local = true","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh = \"TARGET\"","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"user@host","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"~/.ssh/config","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"known_hosts","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"path_prefix","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"codex","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"claude","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh host command","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"/opt/homebrew/bin","chunkId":"hosts#reaching-a-host"}]
sources: [{"chunkId":"hosts#reaching-a-host","url":"/docs/hosts#reaching-a-host","anchor":"reaching-a-host"}]
---

A host is either local, running work on the calling machine with no SSH hop, or reached through an SSH destination such as a user-and-host pair or a config alias. SSH runs in batch mode with a short connect timeout and strict host key checking and never prompts, so set up the host key and key authentication first. The remote side must find hev on a non-interactive PATH; a PATH prefix setting handles tools installed where only login shells look, on Unix hosts only.
