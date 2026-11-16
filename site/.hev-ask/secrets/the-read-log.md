---
id: "secrets#the-read-log"
title: "Secrets"
heading: "The read log"
group: "Operate"
order: 330
url: "/docs/secrets#the-read-log"
anchor: "the-read-log"
terms: ["read","every","real","provider","request","appends","line","private","time","caller","kind","reference","count","labels","outcome","cache","hits","free","events","variable","name","plus","short","hash","never","holds","values","tokens","references","rotates","size","find","heaviest","callers","account","runs","2026","11t21","bash","4121"]
hash: "c7c358a10e1d78420a43b5f853332ec41cd8af575c561eee2c9718ae8b9ce0fc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"2026-11-11T21:31:02Z caller=bash[4121]\u003ehev_secrets_read kind=inject refs=1 labels=DB_PASSWORD#9f2c11ab outcome=ok provider=op\n2026-11-11T21:31:05Z caller=bash[4121]\u003ehev_secrets_run kind=get refs=2 labels=DB_USER#04be71d2,DB_PASSWORD#5a1c9e30 outcome=ok provider=azkv","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"hev secrets status\nawk '{print $2}' ~/.hev/secrets/op-reads.log | sort | uniq -c | sort -rn | head\ngrep 'provider=azkv' ~/.hev/secrets/op-reads.log | grep -c rate_limited   # a throttled vault","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"~/.hev/secrets/op-reads.log","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"cache","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"--caller","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"HEV_SECRETS_CALLER","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"inject","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"read","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"ratelimit","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"get","chunkId":"secrets#the-read-log"},{"kind":"code","literal":"printf %s op://vault/item/field | sha256sum | cut -c1-8","chunkId":"secrets#the-read-log"},{"kind":"value","literal":"SHA-256","chunkId":"secrets#the-read-log"}]
sources: [{"chunkId":"secrets#the-read-log","url":"/docs/secrets#the-read-log","anchor":"the-read-log"}]
---

Every real provider request appends a line to a private read log with time, caller, request kind, reference count, labels, outcome and provider; cache hits add free events. Labels are the variable name plus a short hash of the reference, the log never holds values, tokens or references, it rotates by size, and it is how you find the heaviest callers when an account runs out.
