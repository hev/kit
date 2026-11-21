---
id: "review"
title: "Review"
heading: null
group: "Use"
order: 356
url: "/docs/review"
anchor: null
terms: ["review","opens","dispatched","change","hunk","terminal","diff","viewer","modem","worker","reviewers","notes","next","lines","send","delivers","anchored","message","fetch","merge","open","print","json","show","items","deliver","curl","fssl","https","install","hunkdiff","brew","stat","callout","astro","codetabs","agentprompt","linkgrid","github","beside"]
hash: "92cb359bc810b581afdd17a7ad02f7de9bd1f9122c572b4b23b4214d3df2450e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev review fix-ci                    # fetch, merge notes, open hunk\nhev review fix-ci --with fix-ci-review-2\nhev review fix-ci --print-notes \u003e notes.json\nhev review send fix-ci --dry-run     # show your notes as anchored items\nhev review send fix-ci               # deliver them to the run","chunkId":"review"},{"kind":"code","literal":"curl -fsSL https://hunk.dev/install.sh | sh     # or: npm i -g hunkdiff, brew install hunk","chunkId":"review"},{"kind":"code","literal":"hev review","chunkId":"review"},{"kind":"code","literal":"hev review send","chunkId":"review"},{"kind":"code","literal":"git diff --stat","chunkId":"review"},{"kind":"value","literal":"Callout.astro","chunkId":"review"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"review"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"review"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"review"},{"kind":"value","literal":"hunk.dev","chunkId":"review"},{"kind":"value","literal":"github.com","chunkId":"review"}]
sources: [{"chunkId":"review","url":"/docs/review","anchor":null}]
---

hev review opens a dispatched run's change in hunk (a terminal diff viewer by modem-dev) with the worker's and reviewers' notes next to the lines; you add your own notes and hev review send delivers them to the run as one anchored message. kit has no diff viewer of its own.
