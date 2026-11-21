---
id: "api/traces#search"
title: "Traces API"
heading: "Search"
group: "API"
order: 68
url: "/docs/api/traces#search"
anchor: "search"
terms: ["search","across","transcript","chunks","evaluations","ranked","session","query","extra","phrasings","filters","cursor","curl","https","hevmind","traces","authorization","bearer","data","urlencode","docs","build","fail","also","astro","broken","link","since","mode","hybrid","phrase","role","user","assistant","system","until","harness","model","host","project"]
hash: "2d488751d30d035c04fbcc73d69227d31890b40dde2e69e17dd7f25b2f40c924"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -G https://kit.hevmind.com/v1/traces/search \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  --data-urlencode \"q=why did the docs build fail\" \\\n  --data-urlencode \"also=astro build broken link\" \\\n  --data-urlencode \"since=14d\"","chunkId":"api/traces#search"},{"kind":"code","literal":"also","chunkId":"api/traces#search"},{"kind":"code","literal":"mode","chunkId":"api/traces#search"},{"kind":"code","literal":"hybrid","chunkId":"api/traces#search"},{"kind":"code","literal":"phrase","chunkId":"api/traces#search"},{"kind":"code","literal":"role","chunkId":"api/traces#search"},{"kind":"code","literal":"user","chunkId":"api/traces#search"},{"kind":"code","literal":"assistant","chunkId":"api/traces#search"},{"kind":"code","literal":"system","chunkId":"api/traces#search"},{"kind":"code","literal":"since","chunkId":"api/traces#search"},{"kind":"code","literal":"until","chunkId":"api/traces#search"},{"kind":"code","literal":"harness","chunkId":"api/traces#search"},{"kind":"code","literal":"model","chunkId":"api/traces#search"},{"kind":"code","literal":"host","chunkId":"api/traces#search"},{"kind":"code","literal":"project","chunkId":"api/traces#search"},{"kind":"code","literal":"skill","chunkId":"api/traces#search"},{"kind":"code","literal":"tool","chunkId":"api/traces#search"},{"kind":"code","literal":"branch","chunkId":"api/traces#search"},{"kind":"code","literal":"TraceSearchHit","chunkId":"api/traces#search"},{"kind":"code","literal":"session","chunkId":"api/traces#search"},{"kind":"code","literal":"snippet","chunkId":"api/traces#search"},{"kind":"code","literal":"turn_uuid","chunkId":"api/traces#search"},{"kind":"code","literal":"block_type","chunkId":"api/traces#search"},{"kind":"code","literal":"tool_name","chunkId":"api/traces#search"}]
sources: [{"chunkId":"api/traces#search","url":"/docs/api/traces#search","anchor":"search"}]
---

One search across transcript chunks and evaluations, ranked per session, with query, extra phrasings, filters and cursor.
