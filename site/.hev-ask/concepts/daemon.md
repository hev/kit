---
id: "concepts#daemon"
title: "Concepts"
heading: "Daemon"
group: "Use"
order: 101
url: "/docs/concepts#daemon"
anchor: "daemon"
terms: ["daemon","capture","scans","claude","code","codex","transcripts","every","five","minutes","scrubs","secrets","indexes","files","layer","namespace","dispatch","does","depend","runs","started","locally","supervised","separately","their","traces","indexed","once","scanned","hevd","index","hevlayer","machine","archive","installs","macos","linux","foreground","anywhere","shows"]
hash: "46bfc5497a0eb6d12c718479410f892816cc61b7d1046d063e614049c3f22919"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev up","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev d","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev s","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev index","chunkId":"concepts#daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"concepts#daemon"}]
sources: [{"chunkId":"concepts#daemon","url":"/docs/concepts#daemon","anchor":"daemon"}]
---

The capture daemon scans Claude Code and Codex transcripts every five minutes, scrubs secrets and indexes them into files or a hev layer namespace. Dispatch does not depend on it; runs are started over SSH or locally and supervised separately, and their traces are indexed once scanned.
