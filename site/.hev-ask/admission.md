---
id: "admission"
title: "Admission"
heading: null
group: "Operate"
order: 0
url: "/docs/admission"
anchor: null
terms: ["receiving","host","keeps","list","repositories","work","against","dispatch","config","senders","cannot","override","bypass","order","repo","does","admit","refused","before","anything","fetched","branched","guard","people","shell","access","account","callout","astro","codetabs","agentprompt","decides","itself","repos","receives","limit","accepts","lives","requests","replace"]
hash: "03c541380a339c2eacd05d866e3fbf6513512ba12752bc77280ad8a810164983"
mode: "agent-primary"
facts: [{"kind":"value","literal":"Callout.astro","chunkId":"admission"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"admission"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"admission"}]
sources: [{"chunkId":"admission","url":"/docs/admission","anchor":null}]
---

Each receiving host keeps its own list of the repositories it will run work against, in its own dispatch config; senders cannot override or bypass it. An order for a repo the host does not admit is refused before anything is fetched or branched. It is not a guard against people with shell access to the host account.
