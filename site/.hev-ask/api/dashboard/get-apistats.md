---
id: "api/dashboard#get-apistats"
title: "Dashboard API"
heading: "GET /api/stats"
group: "API"
order: 46
url: "/docs/api/dashboard#get-apistats"
anchor: "get-apistats"
terms: ["stats","returns","totals","charts","matching","traces","cost","view","trace","prompt","tool","token","counts","spend","model","wall","hours","cache","rate","weekday","hour","heatmap","daily","project","series","curl","http","8787","window","timezone","grid","days","generated","aggregation","latest","rows","prompts","tools","tokens","coverage"]
hash: "6fdcd7d697ef2fc7fe24031e4ee5e964041561eba8e290883e0a7009d171212b"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s 'http://127.0.0.1:8787/api/stats?window=7d'","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"timezone","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"prompt_grid","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"window","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"days","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"all","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"generated_at","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"aggregation","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"kit_latest_rows","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"traces","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"prompts","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"tools","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"tokens","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"spend","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"cost_per_prompt","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"api_hours","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"wall_hours","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"cache_hit","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"[weekday][hour]","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"prompt_coverage","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"with","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"total","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"daily","chunkId":"api/dashboard#get-apistats"},{"kind":"code","literal":"day","chunkId":"api/dashboard#get-apistats"}]
sources: [{"chunkId":"api/dashboard#get-apistats","url":"/docs/api/dashboard#get-apistats","anchor":"get-apistats"}]
---

Returns totals and charts for matching traces (the Cost view): trace, prompt, tool and token counts, spend, cost per prompt, model and wall hours, cache hit rate, a weekday-by-hour prompt heatmap, daily and per-model spend, and per-project cost series.
