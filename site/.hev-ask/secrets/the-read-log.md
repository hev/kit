---
id: "secrets#the-read-log"
title: "Secrets"
heading: "The read log"
group: "Operate"
order: 262
url: "/docs/secrets#the-read-log"
anchor: "the-read-log"
terms: ["read","every","real","secrets","request","cache","hits","appends","line","time","caller","kind","reference","count","hashed","labels","outcome","provider","never","holds","values","tokens","references","rotates","quota","runs","status","simple","text","processing","find","heaviest","callers","throttled","vault","requests","without","deleting","anything","directory"]
hash: "6be053ff2432e77e905170438ae354f2c7f9473db47129acf3417ff9b5ba5e18"
mode: "agent-primary"
facts: [{"kind":"code","literal":"2026-11-11T21:31:02Z caller=bash[4121]\u003ehev_secrets_read kind=inject refs=1 labels=DB_PASSWORD#9f2c11ab outcome=ok provider=op\n2026-11-11T21:31:05Z caller=bash[4121]\u003ehev_secrets_run kind=get refs=2 labels=DB_USER#04be71d2,DB_PASSWORD#5a1c9e30 outcome=ok provider=azkv","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"hev secrets status\nawk '{print $2}' ~/.hev/secrets/op-reads.log | sort | uniq -c | sort -rn | head\ngrep 'provider=azkv' ~/.hev/secrets/op-reads.log | grep -c rate_limited   # a throttled vault","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"~/.hev/secrets/op-reads.log","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"--caller","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"HEV_SECRETS_CALLER","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"inject","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"read","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"ratelimit","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"get","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"printf %s op://vault/item/field | sha256sum | cut -c1-8","chunkId":"secrets#the-read-log"},{"kind":"value","literal":"SHA-256","chunkId":"secrets#the-read-log"}]
sources: [{"chunkId":"secrets#the-read-log","url":"/docs/secrets#the-read-log","anchor":"the-read-log"}]
---

Every real secrets request, but not cache hits, appends a line to a read log with UTC time, caller, request kind, reference count, hashed labels, outcome and provider; it never holds values, tokens or references and rotates at 4 MiB. When the quota runs out, use status and simple text processing on the log to find the heaviest callers and throttled Key Vault requests, without deleting anything in the secrets directory.
