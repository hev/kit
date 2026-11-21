---
id: "api/traces#list-sessions"
title: "Traces API"
heading: "List sessions"
group: "API"
order: 66
url: "/docs/api/traces#list-sessions"
anchor: "list-sessions"
terms: ["list","sessions","lists","ordered","time","descending","session","rows","same","filters","apply","search","stats","values","curl","https","hevmind","traces","harness","claude","since","limit","authorization","bearer","sessionrow","until","model","host","project","repo","skill","tool","exclude","branch","order","poor","true","false","hevapikey","come"]
hash: "820f2841f2eedf79a76146c9a6d14d114f8d12f0246294b5f22837bc7e0a7237"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"https://kit.hevmind.com/v1/traces/sessions?harness=claude\u0026since=7d\u0026limit=20\" \\\n  -H \"Authorization: Bearer $HEV_API_KEY\"","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"end","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"id","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"SessionRow","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"since","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"until","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"harness","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"model","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"host","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"project","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"repo_url","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"skill","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"tool","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"exclude_harness","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"exclude_model","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"exclude_host","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"exclude_project","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"branch","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"order","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"poor","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"true","chunkId":"api/traces#list-sessions"},{"kind":"code","literal":"false","chunkId":"api/traces#list-sessions"}]
sources: [{"chunkId":"api/traces#list-sessions","url":"/docs/api/traces#list-sessions","anchor":"list-sessions"}]
---

Lists sessions ordered by end time descending as session rows; the same filters apply to list, search, stats and values.
