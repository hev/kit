---
id: "secrets#3-read-a-1password-secret"
title: "Secrets"
heading: "3. Read a 1Password secret"
group: "Operate"
order: 244
url: "/docs/secrets#3-read-a-1password-secret"
anchor: "3-read-a-1password-secret"
terms: ["read","1password","secret","shell","variable","through","secrets","label","explain","output","checking","only","empty","line","should","report","daemon","exit","retry","time","stop","never","fall","back","reading","directly","password","vault","item","field","dbpassword","reference","give","place","print","echo","value","check","example","test"]
hash: "15aebeb09352b8bcdf5c21d99d48075bc8fb1521adbd7076350aa2dfbf749d61"
mode: "agent-primary"
facts: [{"kind":"code","literal":"DB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain op://VAULT/ITEM/FIELD)","chunkId":"secrets#3-read-a-1password-secret"}]
sources: [{"chunkId":"secrets#3-read-a-1password-secret","url":"/docs/secrets#3-read-a-1password-secret","anchor":"3-read-a-1password-secret"}]
---

Read one 1Password secret into a shell variable through hev secrets with a label, TTL and explain output, checking only that it is non-empty. The explain line should report via the daemon; on exit 75 report the retry time and stop, and never fall back to reading with op directly.
