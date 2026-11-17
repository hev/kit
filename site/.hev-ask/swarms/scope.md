---
id: "swarms#scope"
title: "Swarms"
heading: "Scope"
group: "Use"
order: 360
url: "/docs/swarms#scope"
anchor: "scope"
terms: ["scope","swarm","orchestrator","scoped","runs","whose","metadata","records","name","field","admission","writes","identity","order","frontmatter","other","callers","cannot","stop","message","retry","rework","only","authority","swarms","matches","starts","another","caller","claim"]
hash: "dc2b9892de2aa208e6675a5b813f678cd7e741fffe6adce06ff7ff0b2a2afb95"
mode: "agent-primary"
facts: [{"kind":"code","literal":"swarm=NAME","chunkId":"swarms#scope"},{"kind":"code","literal":"swarm: NAME","chunkId":"swarms#scope"},{"kind":"code","literal":"stop","chunkId":"swarms#scope"}]
sources: [{"chunkId":"swarms#scope","url":"/docs/swarms#scope","anchor":"scope"}]
---

A swarm's orchestrator is scoped to runs whose metadata records that swarm's name, a field admission writes from the swarm's identity and that order frontmatter or other callers cannot set. So a swarm can stop, message, retry or rework only its own runs, and stop is an authority swarms add.
