---
id: "admission#what-is-matched"
title: "Admission"
heading: "What is matched"
group: "Operate"
order: 5
url: "/docs/admission#what-is-matched"
anchor: "what-is-matched"
terms: ["matched","host","compares","clone","origin","order","repo","label","against","entry","https","spellings","same","match","user","transport","trailing","slash","suffix","ignored","server","names","case","insensitive","paths","sensitive","ports","must","local","windows","supported","unreadable","refused","remote","github","acme","runs","field","only","find"]
hash: "52945b7165c7633d780cdf1df98f425ac18428ad57983c66294e829bd3032644"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git remote get-url origin","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"repo","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"git@github.com:acme/api","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"https://github.com/acme/api.git","chunkId":"admission#what-is-matched"},{"kind":"code","literal":".git","chunkId":"admission#what-is-matched"}]
sources: [{"chunkId":"admission#what-is-matched","url":"/docs/admission#what-is-matched","anchor":"what-is-matched"}]
---

The host compares its own clone's origin URL, not the order's repo label, against each entry. SSH and HTTPS spellings of the same repo match; user, transport, trailing slash and .git suffix are ignored, server names are case insensitive but paths are case sensitive, and ports must match. Local and Windows paths are supported, and an unreadable origin is refused.
