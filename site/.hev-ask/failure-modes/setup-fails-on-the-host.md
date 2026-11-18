---
id: "failure-modes#setup-fails-on-the-host"
title: "Failure modes"
heading: "Setup fails on the host"
group: "Operate"
order: 164
url: "/docs/failure-modes#setup-fails-on-the-host"
anchor: "setup-fails-on-the-host"
terms: ["setup","fails","host","after","admission","fetches","origin","creates","worktree","branch","either","step","output","comes","back","recorded","failed","common","causes","repo","cloned","under","workspace","clone","missing","base","credentials","cannot","fetch","cause","clean","send","again","error","exit","status","fatal","invalid","reference","release"]
hash: "5ba6c3d8525fd83eccdf906ba5a8595a41e63e74b91fb2d3371b535e34519221"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: git: exit status 128: fatal: invalid reference: origin/release","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"d/NAME","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin/BASE","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#setup-fails-on-the-host"}]
sources: [{"chunkId":"failure-modes#setup-fails-on-the-host","url":"/docs/failure-modes#setup-fails-on-the-host","anchor":"setup-fails-on-the-host"}]
---

After admission the host fetches origin and creates the worktree and branch; if either Git step fails, Git's output comes back and the run is recorded as failed. Common causes are a repo not cloned under the workspace, a clone with no origin, a missing base branch, or Git credentials that cannot fetch. Fix the cause, clean the run and send it again.
