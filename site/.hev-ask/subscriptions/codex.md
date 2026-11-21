---
id: "subscriptions#codex"
title: "Subscriptions"
heading: "Codex"
group: "Operate"
order: 409
url: "/docs/subscriptions#codex"
anchor: "codex"
terms: ["codex","readings","need","network","call","come","rate","limit","data","records","local","session","rollouts","reading","only","most","recent","files","payload","limits","token","count","home","sessions","yyyy","jsonl","window","minutes","primary","secondary","plan","type","credits","name","resets","rolled","auth","json","chatgpt","login"]
hash: "bef8080a725428db167c43f7151cbdcb061b051c2d09b73c3d7b451699736c07"
mode: "agent-primary"
facts: [{"kind":"code","literal":"payload.rate_limits","chunkId":"subscriptions#codex"},{"kind":"code","literal":"token_count","chunkId":"subscriptions#codex"},{"kind":"code","literal":"$CODEX_HOME/sessions/YYYY/MM/DD/*.jsonl","chunkId":"subscriptions#codex"},{"kind":"code","literal":"CODEX_HOME","chunkId":"subscriptions#codex"},{"kind":"code","literal":"~/.codex","chunkId":"subscriptions#codex"},{"kind":"code","literal":"limit_id","chunkId":"subscriptions#codex"},{"kind":"code","literal":"window_minutes","chunkId":"subscriptions#codex"},{"kind":"code","literal":"primary","chunkId":"subscriptions#codex"},{"kind":"code","literal":"secondary","chunkId":"subscriptions#codex"},{"kind":"code","literal":"plan_type","chunkId":"subscriptions#codex"},{"kind":"code","literal":"credits","chunkId":"subscriptions#codex"},{"kind":"code","literal":"codex","chunkId":"subscriptions#codex"},{"kind":"code","literal":"limit_name","chunkId":"subscriptions#codex"},{"kind":"code","literal":"resets_at","chunkId":"subscriptions#codex"},{"kind":"code","literal":"rolled over","chunkId":"subscriptions#codex"},{"kind":"code","literal":"auth.json","chunkId":"subscriptions#codex"},{"kind":"code","literal":"chatgpt","chunkId":"subscriptions#codex"},{"kind":"code","literal":"login expired","chunkId":"subscriptions#codex"},{"kind":"code","literal":"internal/cost","chunkId":"subscriptions#codex"}]
sources: [{"chunkId":"subscriptions#codex","url":"/docs/subscriptions#codex","anchor":"codex"}]
---

Codex readings need no network call: they come from rate-limit data Codex records in its local session rollouts, reading only the most recent few files.
