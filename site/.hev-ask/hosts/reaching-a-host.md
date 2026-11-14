---
id: "hosts#reaching-a-host"
title: "Hosts"
heading: "Reaching a host"
group: "Operate"
order: 175
url: "/docs/hosts#reaching-a-host"
anchor: "reaching-a-host"
terms: ["reaching","host","either","local","reached","destination","alias","never","both","runs","interactively","strict","checking","keys","first","must","interactive","path","unix","hosts","extend","prefixes","ignored","windows","true","dispatch","target","user","config","known","prefix","codex","claude","command","homebrew","exactly","work","machine","calls","passed"]
hash: "a82a8f8be6214677e3367cedf64d70b607ed1c49df535ef17d205c7964a07000"
mode: "agent-primary"
facts: [{"kind":"code","literal":"local = true","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh = \"TARGET\"","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"user@host","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"~/.ssh/config","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"known_hosts","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"hev","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"path_prefix","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"codex","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"claude","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"ssh host command","chunkId":"hosts#reaching-a-host"},{"kind":"code","literal":"/opt/homebrew/bin","chunkId":"hosts#reaching-a-host"}]
sources: [{"chunkId":"hosts#reaching-a-host","url":"/docs/hosts#reaching-a-host","anchor":"reaching-a-host"}]
---

A host is either local or reached over SSH by a destination or alias, never both. SSH runs non-interactively with strict host key checking, so set up keys first; hev must be on the non-interactive SSH PATH, which Unix hosts can extend with path prefixes (ignored on Windows).
