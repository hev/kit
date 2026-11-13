---
id: "failure-modes#the-host-refuses-the-repo"
title: "Failure modes"
heading: "The host refuses the repo"
group: "Operate"
order: 113
url: "/docs/failure-modes#the-host-refuses-the-repo"
anchor: "the-host-refuses-the-repo"
terms: ["host","refuses","repo","admission","list","does","include","clone","origin","cannot","read","sender","override","owner","send","work","elsewhere","error","acme","admitted","buildbox","dispatch","config","target","receiving","also","whose","whoever","runs","hevdispatchconfig","points","file","every","order","fails"]
hash: "6f47cc86b67cb68051680ff8792017c9a1e2a903be7dfa49621a23e419b23838"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"[admission]","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"origin","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"failure-modes#the-host-refuses-the-repo"},{"kind":"code","literal":"target admission config: ...","chunkId":"failure-modes#the-host-refuses-the-repo"}]
sources: [{"chunkId":"failure-modes#the-host-refuses-the-repo","url":"/docs/failure-modes#the-host-refuses-the-repo","anchor":"the-host-refuses-the-repo"}]
---

The host's admission list does not include the clone's origin, or it cannot read the origin. The sender cannot override this, so ask the host's owner to add the repo or send the work elsewhere.
