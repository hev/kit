---
id: "tradeoffs#workers-run-without-approval-prompts"
title: "Tradeoffs"
heading: "Workers run without approval prompts"
group: "Reference"
order: 355
url: "/docs/tradeoffs#workers-run-without-approval-prompts"
anchor: "workers-run-without-approval-prompts"
terms: ["workers","without","approval","prompts","unattended","approvals","sandboxing","disabled","both","harnesses","anything","host","user","isolation","dedicated","machine","account","minimal","credentials","repo","admission","worktrees","prevent","collisions","security","boundary","dangerously","bypass","sandbox","skip","permissions","codex","harness","starts","claude","worker","command","cost","give","only"]
hash: "6aad049417cbbcc634a43fbdb8d8efcdab6d83f848778fc26b84768a4e2c440e"
mode: "source-primary"
facts: [{"kind":"code","literal":"--dangerously-bypass-approvals-and-sandbox","chunkId":"tradeoffs#workers-run-without-approval-prompts"},{"kind":"code","literal":"--dangerously-skip-permissions","chunkId":"tradeoffs#workers-run-without-approval-prompts"}]
sources: [{"chunkId":"tradeoffs#workers-run-without-approval-prompts","url":"/docs/tradeoffs#workers-run-without-approval-prompts","anchor":"workers-run-without-approval-prompts"}]
---

Workers run unattended with approvals and sandboxing disabled in both harnesses, so they can do anything the host user can. Isolation is the host's job: use a dedicated machine or account, minimal credentials and repo admission; per-run worktrees prevent collisions but are not a security boundary.
