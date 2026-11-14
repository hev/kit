---
id: "api/dashboard#trace-filters"
title: "Dashboard API"
heading: "Trace filters"
group: "API"
order: 48
url: "/docs/api/dashboard#trace-filters"
anchor: "trace-filters"
terms: ["trace","filters","sessions","stats","search","values","endpoints","share","dashboard","filter","parameters","time","window","project","model","harness","host","includes","excludes","skills","tools","ranges","tool","calls","tokens","cost","wall","start","dates","summary","presence","heatmap","cell","timezone","grade","repeated","ored","different","anded","match"]
hash: "0fffb905c6ba3beaff37b6a299ce8de681839fdd937596dc9f1512a170c47453"
mode: "source-primary"
facts: [{"kind":"code","literal":"/api/sessions","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"/api/stats","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"/api/search","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"/api/values","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"window","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"7d","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"30d","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"90d","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"all","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"project","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"model","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"harness","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"host","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"widgets","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_project","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_model","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_harness","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_host","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"skill","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tool","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tools_min","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tools_max","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tokens_min","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tokens_max","chunkId":"api/dashboard#trace-filters"}]
sources: [{"chunkId":"api/dashboard#trace-filters","url":"/docs/api/dashboard#trace-filters","anchor":"trace-filters"}]
---

The sessions, stats, search and values endpoints share the dashboard's filter bar parameters: time window, project/model/harness/host includes and excludes, skills and tools used, ranges for tool calls, tokens, cost and wall time, start dates, summary presence, heatmap cell with timezone, and grade filters. Repeated values are ORed, different parameters ANDed; grade filters match nothing with the file archive.
