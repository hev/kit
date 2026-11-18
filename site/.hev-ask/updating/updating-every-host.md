---
id: "updating#updating-every-host"
title: "Updating and channels"
heading: "Updating every host"
group: "Operate"
order: 399
url: "/docs/updating#updating-every-host"
anchor: "updating-every-host"
terms: ["updating","every","host","machine","dispatch","version","channel","whether","behind","preview","would","change","update","hosts","newest","build","edge","named","runs","itself","reports","before","after","versions","only","owner","request","carries","just","fetches","verifies","know","reply","error","must","moved","hand","once","adjacent","builds"]
hash: "d97627d7cd9b807a81e7ed62b11a103fcff6e6c13a707baa0f63e0a23463d956"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch hosts","chunkId":"updating#updating-every-host"}]
sources: [{"chunkId":"updating#updating-every-host","url":"/docs/updating#updating-every-host","anchor":"updating-every-host"}]
---

From the machine you dispatch from you can see each host's version, channel and whether it is behind, preview what would change, and update all hosts or one host to the newest build on its channel, to edge, or to a named version. Each host runs the update on itself and reports before and after versions or why it did not update. Only a host's owner can request this; the request carries just a channel and a version and the host fetches and verifies the build itself. Hosts too old to know the request reply with an error and must be moved by hand once, and adjacent builds stay compatible because hosts update at different moments.
