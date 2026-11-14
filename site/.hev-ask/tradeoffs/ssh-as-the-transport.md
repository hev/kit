---
id: "tradeoffs#ssh-as-the-transport"
title: "Tradeoffs"
heading: "SSH as the transport"
group: "Reference"
order: 350
url: "/docs/tradeoffs#ssh-as-the-transport"
anchor: "ssh-as-the-transport"
terms: ["transport","remote","calls","dispatch","daemon","port","tokens","costs","keys","host","outside","strict","checking","dropped","connections","leave","outcomes","unknown","retried","call","connection","parallel","second","limit","interactive","need","path","prefix","find","tools","show","local","every","request","stdin","json","stdout","there","setup","token"]
hash: "eb17e610f9cf061cbd665ed49855b31d77c105683d6889ca17a38fd65dd01e1e"
mode: "source-primary"
facts: [{"kind":"code","literal":"ssh HOST hev dispatch rpc","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"show","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"~/.local/bin","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"path_prefix","chunkId":"tradeoffs#ssh-as-the-transport"}]
sources: [{"chunkId":"tradeoffs#ssh-as-the-transport","url":"/docs/tradeoffs#ssh-as-the-transport","anchor":"ssh-as-the-transport"}]
---

All remote calls go over SSH to the dispatch RPC with no daemon port, TLS or tokens. Costs: SSH keys and host keys are set up outside hev with strict checking, dropped connections leave outcomes unknown and are not retried, each call is a new connection (parallel with a 12-second limit), and non-interactive SSH may need a path prefix to find tools.
