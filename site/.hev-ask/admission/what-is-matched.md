---
id: "admission#what-is-matched"
title: "Admission"
heading: "What is matched"
group: "Operate"
order: 5
url: "/docs/admission#what-is-matched"
anchor: "what-is-matched"
terms: ["matched","host","reads","origin","clone","compares","list","entry","server","path","https","spellings","repo","match","while","user","names","trailing","slashes","ignored","case","insensitive","paths","sensitive","ports","must","remote","github","acme","runs","field","order","only","find","renaming","directory","sender","different","label","does"]
hash: "52945b7165c7633d780cdf1df98f425ac18428ad57983c66294e829bd3032644"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git remote get-url origin","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"repo","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"git@github.com:acme/api","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"https://github.com/acme/api.git","chunkId":"admission#what-is-matched"},{"kind":"code","literal":".git","chunkId":"admission#what-is-matched"}]
sources: [{"chunkId":"admission#what-is-matched","url":"/docs/admission#what-is-matched","anchor":"what-is-matched"}]
---

The host reads the origin URL of its own clone and compares it to each list entry by server and path. SSH and HTTPS spellings of one repo match, while user names, trailing slashes and a trailing .git are ignored. Server names are case insensitive, paths are case sensitive, and ports must match.
