---
id: "admission#what-is-matched"
title: "Admission"
heading: "What is matched"
group: "Operate"
order: 5
url: "/docs/admission#what-is-matched"
anchor: "what-is-matched"
terms: ["matched","host","compares","origin","clone","against","list","entry","order","repo","field","only","locates","sender","side","renames","labels","matter","matching","server","path","https","spellings","match","rules","case","ports","trailing","parts","local","windows","paths","refuses","whose","cannot","read","remote","github","acme","runs"]
hash: "52945b7165c7633d780cdf1df98f425ac18428ad57983c66294e829bd3032644"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git remote get-url origin","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"repo","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"git@github.com:acme/api","chunkId":"admission#what-is-matched"},{"kind":"code","literal":"https://github.com/acme/api.git","chunkId":"admission#what-is-matched"},{"kind":"code","literal":".git","chunkId":"admission#what-is-matched"}]
sources: [{"chunkId":"admission#what-is-matched","url":"/docs/admission#what-is-matched","anchor":"what-is-matched"}]
---

The host compares the origin URL of its own clone against each list entry; the order's repo field only locates the clone, so sender-side renames or labels do not matter. Matching is by server and path, so SSH and HTTPS spellings of one repo match, with rules for case, ports, trailing parts, local and Windows paths. A host with a list refuses a clone whose origin it cannot read.
