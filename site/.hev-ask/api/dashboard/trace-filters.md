---
id: "api/dashboard#trace-filters"
title: "Dashboard API"
heading: "Trace filters"
group: "API"
order: 63
url: "/docs/api/dashboard#trace-filters"
anchor: "trace-filters"
terms: ["trace","filters","shared","filter","parameters","sessions","stats","search","values","endpoints","matching","dashboard","page","urls","carry","covers","time","window","include","exclude","project","model","harness","host","skill","tool","ranges","calls","tokens","cost","wall","date","bounds","summary","presence","heatmap","cell","timezone","grade","parameter"]
hash: "0fffb905c6ba3beaff37b6a299ce8de681839fdd937596dc9f1512a170c47453"
mode: "source-primary"
facts: [{"kind":"code","literal":"/api/sessions","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"/api/stats","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"/api/search","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"/api/values","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"window","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"7d","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"30d","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"90d","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"all","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"project","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"model","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"harness","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"host","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"widgets","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_project","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_model","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_harness","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"exclude_host","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"skill","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tool","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tools_min","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tools_max","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tokens_min","chunkId":"api/dashboard#trace-filters"},{"kind":"code","literal":"tokens_max","chunkId":"api/dashboard#trace-filters"}]
sources: [{"chunkId":"api/dashboard#trace-filters","url":"/docs/api/dashboard#trace-filters","anchor":"trace-filters"}]
---

The shared filter parameters for the sessions, stats, search and values endpoints, matching the dashboard's filter bar so page URLs carry over. Covers the time window, include and exclude filters for project, model, harness and host, skill and tool filters, ranges for tool calls, tokens, cost and wall time, date bounds, summary presence, heatmap cell and timezone, and grade filters. Values of one parameter are ORed and different parameters ANDed; grade filters match nothing with the file archive.
