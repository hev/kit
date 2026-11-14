---
id: "failure-modes#the-host-refuses-the-repo"
title: "Failure modes"
heading: "The host refuses the repo"
group: "Operate"
order: 162
url: "/docs/failure-modes#the-host-refuses-the-repo"
anchor: "the-host-refuses-the-repo"
terms: ["host","refuses","repo","admitted","error","means","admission","list","doesn","include","clone","origin","read","senders","override","owner","send","elsewhere","unreadable","configured","dispatch","config","makes","every","order","fail","acme","buildbox","target","receiving","also","whose","cannot","sender","whoever","runs","work","hevdispatchconfig","points","file"]
hash: "6f47cc86b67cb68051680ff8792017c9a1e2a903be7dfa49621a23e419b23838"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"[admission]","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"origin","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"target admission config: ...","chunkId":"failure-modes#the-host-refuses-the-repo"}]
sources: [{"chunkId":"failure-modes#the-host-refuses-the-repo","url":"/docs/failure-modes#the-host-refuses-the-repo","anchor":"the-host-refuses-the-repo"}]
---

A not-admitted error means the host's admission list doesn't include the clone's origin (or the origin can't be read); senders can't override it, so ask the host owner to add the repo or send elsewhere. An unreadable configured dispatch config makes every order fail.
