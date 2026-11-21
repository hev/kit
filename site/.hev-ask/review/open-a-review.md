---
id: "review#open-a-review"
title: "Review"
heading: "Open a review"
group: "Use"
order: 358
url: "/docs/review#open-a-review"
anchor: "open-a-review"
terms: ["open","review","opening","fetches","branch","head","merge","base","host","bundle","need","pushed","stored","under","merges","notes","runs","those","named","opens","hunk","options","include","repo","printing","print","dispatch","show","refs","author","source","reviewrun","machine","fetch","asks","order","only","commits","missing","same"]
hash: "3a437b68017b02f1f368c7a72eb38c782862ec01f3ae780dc8271bd6337f5d05"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev review RUN [--host H] [--with REVIEW_RUN] [--print-notes] [--repo DIR]","chunkId":"review#open-a-review"},{"kind":"code","literal":"base","chunkId":"review#open-a-review"},{"kind":"code","literal":"hev dispatch show","chunkId":"review#open-a-review"},{"kind":"code","literal":"refs/hev/review/RUN","chunkId":"review#open-a-review"},{"kind":"code","literal":"--repo","chunkId":"review#open-a-review"},{"kind":"code","literal":"RUN-review…","chunkId":"review#open-a-review"},{"kind":"code","literal":"--with","chunkId":"review#open-a-review"},{"kind":"code","literal":"head","chunkId":"review#open-a-review"},{"kind":"code","literal":"author","chunkId":"review#open-a-review"},{"kind":"code","literal":"source","chunkId":"review#open-a-review"},{"kind":"code","literal":"MERGE_BASE...HEAD","chunkId":"review#open-a-review"},{"kind":"code","literal":"--print-notes","chunkId":"review#open-a-review"},{"kind":"code","literal":"--web","chunkId":"review#open-a-review"},{"kind":"value","literal":"0.25","chunkId":"review#open-a-review"}]
sources: [{"chunkId":"review#open-a-review","url":"/docs/review#open-a-review","anchor":"open-a-review"}]
---

Opening a review: it fetches the branch head and merge base from the run's host as a git bundle (branch need not be pushed, stored under a review ref), merges notes from the run and its review runs (or those named with --with), and opens hunk; options include host, repo and printing notes.
