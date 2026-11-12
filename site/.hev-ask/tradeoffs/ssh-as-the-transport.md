---
id: "tradeoffs#ssh-as-the-transport"
title: "Tradeoffs"
heading: "SSH as the transport"
group: "Reference"
order: 296
url: "/docs/tradeoffs#ssh-as-the-transport"
anchor: "ssh-as-the-transport"
terms: ["transport","every","remote","call","dispatch","json","stdio","there","daemon","port","token","costs","host","keys","outside","strict","checking","prompts","dropped","connection","start","stop","clean","leaves","outcome","unknown","does","retry","time","limit","interactive","lacks","login","path","prefix","needed","show","local","request","stdin"]
hash: "eb17e610f9cf061cbd665ed49855b31d77c105683d6889ca17a38fd65dd01e1e"
mode: "source-primary"
facts: [{"kind":"code","literal":"ssh HOST hev dispatch rpc","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"show","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"~/.local/bin","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"path_prefix","chunkId":"tradeoffs#ssh-as-the-transport"}]
sources: [{"chunkId":"tradeoffs#ssh-as-the-transport","url":"/docs/tradeoffs#ssh-as-the-transport","anchor":"ssh-as-the-transport"}]
---

Every remote call is an SSH call to dispatch rpc with JSON over stdio, so there is no daemon port, TLS or token. Costs: host and SSH keys are set up outside hev with strict checking and no prompts; a dropped connection mid start, stop or clean leaves the outcome unknown and hev does not retry; each call is a new connection with a per-host time limit; and non-interactive SSH lacks your login PATH, so a path prefix may be needed.
