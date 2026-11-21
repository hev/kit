---
id: "failure-modes#the-host-refuses-the-repo"
title: "Failure modes"
heading: "The host refuses the repo"
group: "Operate"
order: 225
url: "/docs/failure-modes#the-host-refuses-the-repo"
anchor: "the-host-refuses-the-repo"
terms: ["host","refuses","repo","admitted","means","receiving","admission","list","excludes","clone","origin","sender","override","owner","send","elsewhere","unreadable","config","fails","every","order","error","acme","buildbox","dispatch","target","also","whose","cannot","read","whoever","runs","work","hevdispatchconfig","points","file"]
hash: "6f47cc86b67cb68051680ff8792017c9a1e2a903be7dfa49621a23e419b23838"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"[admission]","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"origin","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"target admission config: ...","chunkId":"failure-modes#the-host-refuses-the-repo"}]
sources: [{"chunkId":"failure-modes#the-host-refuses-the-repo","url":"/docs/failure-modes#the-host-refuses-the-repo","anchor":"the-host-refuses-the-repo"}]
---

A repo not admitted on the host means the receiving host's admission list excludes the clone's origin; the sender can't override it. Ask the host's owner to add it or send elsewhere. An unreadable admission config fails every order.
