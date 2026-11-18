---
id: "concepts#rework"
title: "Concepts"
heading: "Rework"
group: "Use"
order: 112
url: "/docs/concepts#rework"
anchor: "rework"
terms: ["rework","restarts","stopped","same","worktree","branch","archiving","previous","attempt","status","report","result","appending","note","order","sending","message","ended","does","dispatch","name","archives","appends"]
hash: "adfdd1ef8338a433d8016265efb2222774647166730f62fd134fa5ff4d2e2cc9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rework NAME 'note'","chunkId":"concepts#rework"},{"kind":"code","literal":"## Rework","chunkId":"concepts#rework"},{"kind":"code","literal":"say","chunkId":"concepts#rework"}]
sources: [{"chunkId":"concepts#rework","url":"/docs/concepts#rework","anchor":"rework"}]
---

Rework restarts a stopped run in the same worktree and branch, archiving the previous attempt's log, status, report and result and appending a rework note to the order. Sending a message to an ended run does the same.
