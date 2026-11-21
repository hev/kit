---
id: "notifications#webhooks"
title: "Notifications"
heading: "Webhooks"
group: "Use"
order: 284
url: "/docs/notifications#webhooks"
anchor: "webhooks"
terms: ["webhooks","webhook","just","hook","handler","forwards","events","example","posts","needs","ready","notices","curl","exiting","quietly","otherwise","forward","attention","case","exit","esac","content","type","application","json","data","binary","null","hooks","timeout","event","every","notice","keep","environment","hevwebhookurl","write","named","directory","path"]
hash: "e2394e88790afc5876b0341b4885fd578b2f779e0cb1787059dbbc745e6a0ed2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# hev-hook-webhook: forward attention notices to a URL\ncase \"$1\" in needs-you|ready) ;; *) exit 0 ;; esac\n[ -n \"$HEV_WEBHOOK_URL\" ] || exit 0\ncurl -fsS -m 1 -H 'Content-Type: application/json' --data-binary @- \"$HEV_WEBHOOK_URL\" \u003e/dev/null","chunkId":"notifications#webhooks"},{"kind":"code","literal":"needs-you","chunkId":"notifications#webhooks"},{"kind":"code","literal":"ready","chunkId":"notifications#webhooks"},{"kind":"code","literal":"[hooks] timeout","chunkId":"notifications#webhooks"}]
sources: [{"chunkId":"notifications#webhooks","url":"/docs/notifications#webhooks","anchor":"webhooks"}]
---

A webhook is just a hook handler that forwards events; the example posts needs-you and ready notices to a URL from an env var using curl, exiting quietly otherwise.
