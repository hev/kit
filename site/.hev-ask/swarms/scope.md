---
id: "swarms#scope"
title: "Swarms"
heading: "Scope"
group: "Use"
order: 298
url: "/docs/swarms#scope"
anchor: "scope"
terms: ["scope","swarm","orchestrator","matches","only","runs","whose","metadata","records","name","admission","writes","neither","order","frontmatter","other","callers","stop","retry","rework","authority","swarms","field","identity","starts","another","caller","claim"]
hash: "dc2b9892de2aa208e6675a5b813f678cd7e741fffe6adce06ff7ff0b2a2afb95"
mode: "agent-primary"
facts: [{"kind":"code","literal":"swarm=NAME","chunkId":"swarms#scope"},{"kind":"code","literal":"swarm: NAME","chunkId":"swarms#scope"},{"kind":"code","literal":"stop","chunkId":"swarms#scope"}]
sources: [{"chunkId":"swarms#scope","url":"/docs/swarms#scope","anchor":"scope"}]
---

A swarm's orchestrator scope matches only runs whose metadata records that swarm name. Admission writes it, and neither order frontmatter nor other callers can set it, so a swarm can stop, say to, retry or rework only its own runs. Stop is an authority swarms add.
