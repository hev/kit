---
id: "failure-modes#setup-fails-on-the-host"
title: "Failure modes"
heading: "Setup fails on the host"
group: "Operate"
order: 102
url: "/docs/failure-modes#setup-fails-on-the-host"
anchor: "setup-fails-on-the-host"
terms: ["setup","fails","host","fetching","origin","creating","worktree","recorded","failed","error","common","causes","missing","clone","under","workspace","base","branch","credentials","cannot","fetch","clean","resend","exit","status","fatal","invalid","reference","release","name","dispatch","after","admission","fetches","creates","either","step","comes","back","output"]
hash: "5ba6c3d8525fd83eccdf906ba5a8595a41e63e74b91fb2d3371b535e34519221"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: git: exit status 128: fatal: invalid reference: origin/release","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"d/NAME","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin/BASE","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#setup-fails-on-the-host"}]
sources: [{"chunkId":"failure-modes#setup-fails-on-the-host","url":"/docs/failure-modes#setup-fails-on-the-host","anchor":"setup-fails-on-the-host"}]
---

If fetching origin or creating the worktree fails, the run is recorded failed with Git's error. Common causes are a missing clone under the workspace, no origin, a missing base branch, or credentials that cannot fetch; fix it, clean, and resend.
