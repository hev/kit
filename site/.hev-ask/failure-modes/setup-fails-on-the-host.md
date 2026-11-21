---
id: "failure-modes#setup-fails-on-the-host"
title: "Failure modes"
heading: "Setup fails on the host"
group: "Operate"
order: 220
url: "/docs/failure-modes#setup-fails-on-the-host"
anchor: "setup-fails-on-the-host"
terms: ["setup","fails","host","after","admission","fetch","worktree","creation","invalid","reference","origin","release","recorded","failed","output","causes","repo","cloned","under","workspace","missing","base","branch","credentials","clean","resend","error","exit","status","fatal","name","dispatch","fetches","creates","either","step","comes","back","common","clone"]
hash: "5ba6c3d8525fd83eccdf906ba5a8595a41e63e74b91fb2d3371b535e34519221"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: git: exit status 128: fatal: invalid reference: origin/release","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"d/NAME","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin/BASE","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#setup-fails-on-the-host"}]
sources: [{"chunkId":"failure-modes#setup-fails-on-the-host","url":"/docs/failure-modes#setup-fails-on-the-host","anchor":"setup-fails-on-the-host"}]
---

If setup fails after admission (fetch or worktree creation, e.g. invalid reference origin/release), the run is recorded failed with Git's output. Causes: repo not cloned under workspace, no origin, missing base branch, or credentials. Fix, clean the run and resend.
