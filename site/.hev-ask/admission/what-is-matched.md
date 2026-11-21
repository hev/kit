---
id: "admission#what-is-matched"
title: "Admission"
heading: "What is matched"
group: "Operate"
order: 5
url: "/docs/admission#what-is-matched"
anchor: "what-is-matched"
terms: ["matched","host","checks","origin","clone","sender","label","directory","name","urls","compare","server","path","https","spellings","same","repo","match","long","entry","transport","rules","remote","github","acme","runs","compares","field","order","only","find","renaming","different","does","change","compared","matches","user","names","trailing"]
hash: "52945b7165c7633d780cdf1df98f425ac18428ad57983c66294e829bd3032644"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git remote get-url origin","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"repo","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"git@github.com:acme/api","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"https://github.com/acme/api.git","chunkId":"admission#what-is-matched"},{"kind":"code","literal":".git","chunkId":"admission#what-is-matched"}]
sources: [{"chunkId":"admission#what-is-matched","url":"/docs/admission#what-is-matched","anchor":"what-is-matched"}]
---

The host checks the origin URL of its own clone, not the sender's label or directory name. URLs compare by server and path, so SSH and HTTPS spellings of the same repo match as long as the entry's transport rules are met.
