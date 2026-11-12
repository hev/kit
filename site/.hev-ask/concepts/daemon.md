---
id: "concepts#daemon"
title: "Concepts"
heading: "Daemon"
group: "Use"
order: 46
url: "/docs/concepts#daemon"
anchor: "daemon"
terms: ["daemon","hevd","capture","scans","claude","code","codex","transcripts","every","five","minutes","scrubs","secrets","indexes","dispatch","does","through","host","take","work","stopped","index","hevlayer","machine","archive","files","layer","namespace","installs","macos","linux","runs","foreground","anywhere","shows","status","started","locally","supervised","process"]
hash: "46bfc5497a0eb6d12c718479410f892816cc61b7d1046d063e614049c3f22919"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev up","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev d","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev s","chunkId":"concepts#daemon"},{"kind":"code","literal":"hev index","chunkId":"concepts#daemon"},{"kind":"value","literal":"hevlayer.com","chunkId":"concepts#daemon"}]
sources: [{"chunkId":"concepts#daemon","url":"/docs/concepts#daemon","anchor":"daemon"}]
---

hevd is the capture daemon that scans Claude Code and Codex transcripts every five minutes, scrubs secrets and indexes them. Dispatch does not go through it, so a host can take work with the daemon stopped.
