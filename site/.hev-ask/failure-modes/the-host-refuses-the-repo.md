---
id: "failure-modes#the-host-refuses-the-repo"
title: "Failure modes"
heading: "The host refuses the repo"
group: "Operate"
order: 107
url: "/docs/failure-modes#the-host-refuses-the-repo"
anchor: "the-host-refuses-the-repo"
terms: ["host","refuses","repo","admitted","error","means","admission","list","lacks","clone","origin","cannot","read","sender","override","owner","send","elsewhere","unreadable","configured","file","makes","every","order","fail","acme","buildbox","dispatch","config","target","receiving","also","whose","whoever","runs","work","hevdispatchconfig","points","fails"]
hash: "6f47cc86b67cb68051680ff8792017c9a1e2a903be7dfa49621a23e419b23838"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"[admission]","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"origin","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"target admission config: ...","chunkId":"failure-modes#the-host-refuses-the-repo"}]
sources: [{"chunkId":"failure-modes#the-host-refuses-the-repo","url":"/docs/failure-modes#the-host-refuses-the-repo","anchor":"the-host-refuses-the-repo"}]
---

A repo not admitted error means the host's admission list lacks the clone's origin or cannot read it; the sender cannot override this, so ask the host owner or send elsewhere. An unreadable configured admission file makes every order fail.
