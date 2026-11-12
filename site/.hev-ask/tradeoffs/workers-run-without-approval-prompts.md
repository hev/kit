---
id: "tradeoffs#workers-run-without-approval-prompts"
title: "Tradeoffs"
heading: "Workers run without approval prompts"
group: "Reference"
order: 301
url: "/docs/tradeoffs#workers-run-without-approval-prompts"
anchor: "workers-run-without-approval-prompts"
terms: ["workers","without","approval","prompts","unattended","harnesses","sandbox","bypass","flags","command","host","user","cost","isolation","dedicated","machine","account","minimal","credentials","admission","limits","worktrees","branches","prevent","file","clashes","security","boundary","dangerously","approvals","skip","permissions","codex","harness","starts","claude","worker","give","only","work"]
hash: "6aad049417cbbcc634a43fbdb8d8efcdab6d83f848778fc26b84768a4e2c440e"
mode: "source-primary"
facts: [{"kind":"code","literal":"--dangerously-bypass-approvals-and-sandbox","chunkId":"tradeoffs#workers-run-without-approval-prompts"},{"kind":"code","literal":"--dangerously-skip-permissions","chunkId":"tradeoffs#workers-run-without-approval-prompts"}]
sources: [{"chunkId":"tradeoffs#workers-run-without-approval-prompts","url":"/docs/tradeoffs#workers-run-without-approval-prompts","anchor":"workers-run-without-approval-prompts"}]
---

Workers run unattended with the harnesses' approval and sandbox bypass flags and can run any command the host user can. The cost is that isolation is the host's job: use a dedicated machine or account with minimal credentials and admission limits. Per-run worktrees and branches prevent file clashes but are not a security boundary.
