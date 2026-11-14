---
id: "admission#what-is-matched"
title: "Admission"
heading: "What is matched"
group: "Operate"
order: 5
url: "/docs/admission#what-is-matched"
anchor: "what-is-matched"
terms: ["matched","host","compares","origin","remote","clone","against","entry","repo","label","order","matching","server","path","https","spellings","same","match","user","names","transport","trailing","slashes","ignored","case","insensitive","paths","sensitive","ports","must","local","windows","supported","list","refuses","whose","cannot","read","github","acme"]
hash: "52945b7165c7633d780cdf1df98f425ac18428ad57983c66294e829bd3032644"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git remote get-url origin","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"repo","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"git@github.com:acme/api","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"https://github.com/acme/api.git","chunkId":"admission#what-is-matched"},{"kind":"code","literal":".git","chunkId":"admission#what-is-matched"}]
sources: [{"chunkId":"admission#what-is-matched","url":"/docs/admission#what-is-matched","anchor":"what-is-matched"}]
---

The host compares the origin remote of its own clone against each entry, not the repo label in the order. Matching is by server and path, so SSH and HTTPS spellings of the same repo match; user names, transport, trailing slashes and a trailing .git are ignored, server names are case-insensitive, paths case-sensitive, and ports must match. Local and Windows paths are supported; a host with a list refuses a clone whose origin it cannot read.
