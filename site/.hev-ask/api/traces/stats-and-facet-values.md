---
id: "api/traces#stats-and-facet-values"
title: "Traces API"
heading: "Stats and facet values"
group: "API"
order: 69
url: "/docs/api/traces#stats-and-facet-values"
anchor: "stats-and-facet-values"
terms: ["stats","facet","values","fold","latest","matching","session","spend","tokens","hours","prompt","grid","daily","buckets","model","project","breakdowns","route","returns","claude","opus","total","truncated","false","traces","aggregation","rows","tool","skill","harness","host","folds","every","kitlatestrows","activity","totals","apihours","promptgrid","sunday","first"]
hash: "3d2014b76852425184b982d66b69c971586c615a8f83c9842763e833876f372e"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"values\": [ { \"v\": \"claude-opus-5-5\", \"n\": 412 }, { \"v\": \"gpt-5.5\", \"n\": 96 } ], \"total\": 2, \"truncated\": false }","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"GET /v1/traces/stats","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"aggregation: kit_latest_rows","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"api_hours","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"prompt_grid","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"tz","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"daily","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"GET /v1/traces/values?facet=model","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"project","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"model","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"tool","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"skill","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"harness","chunkId":"api/traces#stats-and-facet-values"},{"kind":"code","literal":"host","chunkId":"api/traces#stats-and-facet-values"}]
sources: [{"chunkId":"api/traces#stats-and-facet-values","url":"/docs/api/traces#stats-and-facet-values","anchor":"stats-and-facet-values"}]
---

Stats fold the latest row of each matching session into spend, tokens, API hours, a 7-by-24 prompt grid, daily buckets and per-model and per-project breakdowns; a values route returns facet values.
