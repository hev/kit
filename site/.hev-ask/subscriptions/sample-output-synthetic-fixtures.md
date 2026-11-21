---
id: "subscriptions#sample-output-synthetic-fixtures"
title: "Subscriptions"
heading: "Sample output (synthetic fixtures)"
group: "Operate"
order: 413
url: "/docs/subscriptions#sample-output-synthetic-fixtures"
anchor: "sample-output-synthetic-fixtures"
terms: ["sample","output","synthetic","fixtures","list","show","views","name","harness","plan","state","weekly","usage","percent","reset","columns","subscriptions","resets","ultra","signed","claude","3d6h","codex","plus","1d6h","subscription","source","oauth","read","2030","06t12","meter","note","session","06t15","09t18","stale","spend","toml","login"]
hash: "60b83c9b0bc9d988c8616f3e5a877eac83f4afcf102672ec0ea2c66c01c223dc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$ hev subscriptions\nNAME    HARNESS  PLAN   STATE      WEEKLY  RESETS   AGE\nagy     agy      ultra  signed in  ?       -        30m\nclaude  claude   max    signed in  44%     in 3d6h  0s\ncodex   codex    plus   signed in  27%     in 1d6h  1h","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"$ hev subscription claude\nclaude\nharness:   claude\nplan:      max\nstate:     signed in\nsource:    Claude OAuth usage\nread:      2030-03-06T12:00:00Z (0s ago)\n\nMETER         USED  RESETS                          NOTE\nsession (5h)  9%    2030-03-06T15:00:00Z (in 3h)\nweekly        44%   2030-03-09T18:00:00Z (in 3d6h)","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"WEEKLY","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"AGE","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"(stale)","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"show","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"spend:","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"subscriptions.toml","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"signed in","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"not signed in","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"login expired","chunkId":"subscriptions#sample-output-synthetic-fixtures"},{"kind":"code","literal":"unknown","chunkId":"subscriptions#sample-output-synthetic-fixtures"}]
sources: [{"chunkId":"subscriptions#sample-output-synthetic-fixtures","url":"/docs/subscriptions#sample-output-synthetic-fixtures","anchor":"sample-output-synthetic-fixtures"}]
---

Synthetic sample output of the list and show views, with name, harness, plan, state, weekly usage percent, reset and age columns.
