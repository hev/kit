---
id: "notifications#hook-handler-delivery"
title: "Notifications"
heading: "Hook handler delivery"
group: "Use"
order: 278
url: "/docs/notifications#hook-handler-delivery"
anchor: "hook-handler-delivery"
terms: ["hook","handler","delivery","notices","same","name","handlers","harness","events","notice","kind","argument","json","stdin","output","ignored","runs","most","once","raising","host","example","terminal","bell","needs","desk","ring","anything","printf","ready","done","event","discovery","budget","kill","switch","fail","open","rules","gets"]
hash: "1e4dbd16380c6707668938c252c94c474751ca1154627c0b7658306b72dd27ec"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# hev-hook-desk-bell: ring on anything that needs me\n[ \"$1\" = needs-you ] \u0026\u0026 printf '\\a' \u003e /dev/tty","chunkId":"notifications#hook-handler-delivery"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"notifications#hook-handler-delivery"},{"kind":"code","literal":"needs-you","chunkId":"notifications#hook-handler-delivery"},{"kind":"code","literal":"ready","chunkId":"notifications#hook-handler-delivery"},{"kind":"code","literal":"done","chunkId":"notifications#hook-handler-delivery"},{"kind":"code","literal":"HEV_HOOK_EVENT","chunkId":"notifications#hook-handler-delivery"},{"kind":"code","literal":"HEV_HOOK_HARNESS=kit","chunkId":"notifications#hook-handler-delivery"}]
sources: [{"chunkId":"notifications#hook-handler-delivery","url":"/docs/notifications#hook-handler-delivery","anchor":"hook-handler-delivery"}]
---

Notices go to the same hev-hook-NAME handlers as harness events, with the notice kind as argument and env var and the notice JSON on stdin. Output is ignored; a handler runs at most once per notice ID on the raising host. Example is a terminal bell handler for needs-you.
