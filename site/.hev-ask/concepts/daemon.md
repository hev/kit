---
id: "concepts#daemon"
title: "Concepts"
heading: "Daemon"
group: "Use"
order: 102
url: "/docs/concepts#daemon"
anchor: "daemon"
terms: ["daemon","capture","scans","claude","code","codex","transcripts","machine","every","five","minutes","scrubs","secrets","indexes","archive","files","layer","namespace","dispatch","does","through","host","take","work","stopped","though","traces","indexed","only","once","scan","runs","hevd","index","hevlayer","installs","macos","linux","foreground","anywhere"]
hash: "46bfc5497a0eb6d12c718479410f892816cc61b7d1046d063e614049c3f22919"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev up","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev d","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev s","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev index","chunkId":"concepts#daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"concepts#daemon"}]
sources: [{"chunkId":"concepts#daemon","url":"/docs/concepts#daemon","anchor":"daemon"}]
---

The capture daemon scans Claude Code and Codex transcripts on its machine every five minutes, scrubs secrets, and indexes them into the archive, which is files or a hev layer namespace with a key. Dispatch does not go through the daemon, so a host can take work with it stopped, though its traces are indexed only once a scan runs.
