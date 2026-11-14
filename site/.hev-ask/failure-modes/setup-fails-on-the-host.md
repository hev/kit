---
id: "failure-modes#setup-fails-on-the-host"
title: "Failure modes"
heading: "Setup fails on the host"
group: "Operate"
order: 157
url: "/docs/failure-modes#setup-fails-on-the-host"
anchor: "setup-fails-on-the-host"
terms: ["setup","fails","host","fetching","origin","creating","worktree","branch","after","admission","error","returned","recorded","failed","common","causes","repo","cloned","under","workspace","remote","missing","base","credentials","fetch","clean","record","resend","exit","status","fatal","invalid","reference","release","name","dispatch","fetches","creates","either","step"]
hash: "5ba6c3d8525fd83eccdf906ba5a8595a41e63e74b91fb2d3371b535e34519221"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: git: exit status 128: fatal: invalid reference: origin/release","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"d/NAME","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"origin/BASE","chunkId":"failure-modes#setup-fails-on-the-host"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#setup-fails-on-the-host"}]
sources: [{"chunkId":"failure-modes#setup-fails-on-the-host","url":"/docs/failure-modes#setup-fails-on-the-host","anchor":"setup-fails-on-the-host"}]
---

If fetching origin or creating the worktree branch fails after admission, the Git error is returned and the run recorded as failed. Common causes are the repo not cloned under the workspace, no origin remote, a missing base branch, or Git credentials that can't fetch; fix it, clean the record and resend.
