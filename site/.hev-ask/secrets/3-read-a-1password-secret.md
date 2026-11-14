---
id: "secrets#3-read-a-1password-secret"
title: "Secrets"
heading: "3. Read a 1Password secret"
group: "Operate"
order: 296
url: "/docs/secrets#3-read-a-1password-secret"
anchor: "3-read-a-1password-secret"
terms: ["read","1password","secret","shell","variable","through","cache","explain","output","never","printing","should","show","came","daemon","exit","means","rate","limited","fall","back","directly","password","secrets","label","vault","item","field","dbpassword","reference","give","place","print","echo","value","check","only","empty","example","test"]
hash: "15aebeb09352b8bcdf5c21d99d48075bc8fb1521adbd7076350aa2dfbf749d61"
mode: "agent-primary"
facts: [{"kind":"code","literal":"DB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain op://VAULT/ITEM/FIELD)","chunkId":"secrets#3-read-a-1password-secret"}]
sources: [{"chunkId":"secrets#3-read-a-1password-secret","url":"/docs/secrets#3-read-a-1password-secret","anchor":"3-read-a-1password-secret"}]
---

Read one 1Password secret into a shell variable through the cache with a TTL and explain output, never printing it; explain should show it came via the daemon, exit 75 means rate limited, and do not fall back to the op CLI directly.
