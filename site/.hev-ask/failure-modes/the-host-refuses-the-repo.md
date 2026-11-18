---
id: "failure-modes#the-host-refuses-the-repo"
title: "Failure modes"
heading: "The host refuses the repo"
group: "Operate"
order: 169
url: "/docs/failure-modes#the-host-refuses-the-repo"
anchor: "the-host-refuses-the-repo"
terms: ["host","refuses","repo","admitted","error","means","receiving","admission","list","does","include","clone","origin","cannot","read","sender","override","whoever","runs","send","work","elsewhere","configured","file","unreadable","every","order","fails","target","config","acme","buildbox","dispatch","also","whose","hevdispatchconfig","points"]
hash: "6f47cc86b67cb68051680ff8792017c9a1e2a903be7dfa49621a23e419b23838"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"[admission]","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"origin","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"target admission config: ...","chunkId":"failure-modes#the-host-refuses-the-repo"}]
sources: [{"chunkId":"failure-modes#the-host-refuses-the-repo","url":"/docs/failure-modes#the-host-refuses-the-repo","anchor":"the-host-refuses-the-repo"}]
---

A not-admitted error means the receiving host has an admission list that does not include the clone's origin, or it cannot read the clone's origin. The sender cannot override it: ask whoever runs the host to add the repo or send the work elsewhere. If the host's configured admission file is unreadable, every order fails with a target admission config error.
