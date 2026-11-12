---
id: "hosts#reaching-a-host"
title: "Hosts"
heading: "Reaching a host"
group: "Operate"
order: 126
url: "/docs/hosts#reaching-a-host"
anchor: "reaching-a-host"
terms: ["reaching","host","either","local","running","calling","machine","reached","destination","alias","exactly","must","runs","batch","mode","short","connect","timeout","strict","keys","true","dispatch","target","user","config","known","hosts","path","prefix","codex","claude","command","homebrew","work","calls","passed","pair","start","five","second"]
hash: "a82a8f8be6214677e3367cedf64d70b607ed1c49df535ef17d205c7964a07000"
mode: "agent-primary"
facts: [{"kind":"code","literal":"local = true","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh = \"TARGET\"","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"user@host","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"~/.ssh/config","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"known_hosts","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"path_prefix","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"codex","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"claude","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh host command","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"/opt/homebrew/bin","chunkId":"hosts#reaching-a-host"}]
sources: [{"chunkId":"hosts#reaching-a-host","url":"/docs/hosts#reaching-a-host","anchor":"reaching-a-host"}]
---

A host is either local, running on the calling machine, or reached over SSH with a destination or alias. Exactly one must be set, and SSH runs in batch mode with a short connect timeout and strict host keys.
