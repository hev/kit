---
id: "tradeoffs#ssh-as-the-transport"
title: "Tradeoffs"
heading: "SSH as the transport"
group: "Reference"
order: 294
url: "/docs/tradeoffs#ssh-as-the-transport"
anchor: "ssh-as-the-transport"
terms: ["transport","every","remote","dispatch","call","invocation","command","json","host","without","prompt","take","work","daemon","port","tokens","costs","keys","outside","strict","checking","dropped","connection","during","start","stop","clean","leaves","outcome","unknown","retry","second","limit","listing","interactive","path","need","prefix","show","local"]
hash: "eb17e610f9cf061cbd665ed49855b31d77c105683d6889ca17a38fd65dd01e1e"
mode: "source-primary"
facts: [{"kind":"code","literal":"ssh HOST hev dispatch rpc","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"show","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"~/.local/bin","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"path_prefix","chunkId":"tradeoffs#ssh-as-the-transport"}]
sources: [{"chunkId":"tradeoffs#ssh-as-the-transport","url":"/docs/tradeoffs#ssh-as-the-transport","anchor":"ssh-as-the-transport"}]
---

Every remote dispatch call is an SSH invocation of the dispatch RPC command with JSON in and out, so any host you can SSH to without a prompt can take work, with no daemon port, TLS or tokens. The costs: keys and host keys are set up outside hev with strict checking, a dropped connection during start, stop or clean leaves the outcome unknown and hev will not retry, each call is a new connection (12 second limit per host when listing), and the non-interactive SSH PATH may need a prefix.
