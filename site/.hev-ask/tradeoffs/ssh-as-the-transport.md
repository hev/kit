---
id: "tradeoffs#ssh-as-the-transport"
title: "Tradeoffs"
heading: "SSH as the transport"
group: "Reference"
order: 389
url: "/docs/tradeoffs#ssh-as-the-transport"
anchor: "ssh-as-the-transport"
terms: ["transport","every","remote","call","goes","json","dispatch","needs","daemon","port","setup","token","passwordless","access","enough","send","work","costs","host","keys","outside","strict","checking","prompts","dropped","connection","leaves","outcome","unknown","retried","automatically","fresh","time","limit","interactive","environment","lacks","login","shell","path"]
hash: "eb17e610f9cf061cbd665ed49855b31d77c105683d6889ca17a38fd65dd01e1e"
mode: "source-primary"
facts: [{"kind":"code","literal":"ssh HOST hev dispatch rpc","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"show","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"~/.local/bin","chunkId":"tradeoffs#ssh-as-the-transport"},{"kind":"code","literal":"path_prefix","chunkId":"tradeoffs#ssh-as-the-transport"}]
sources: [{"chunkId":"tradeoffs#ssh-as-the-transport","url":"/docs/tradeoffs#ssh-as-the-transport","anchor":"ssh-as-the-transport"}]
---

Every remote call goes over SSH with JSON in and out, so dispatch needs no daemon port, TLS setup or token: passwordless SSH access is enough to send work. The costs are that SSH and host keys are set up outside hev with strict checking and no prompts, a dropped connection leaves an outcome unknown and is not retried automatically, each call is a fresh connection with a per-host time limit, and the non-interactive SSH environment lacks your login shell's path.
