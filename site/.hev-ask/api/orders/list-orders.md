---
id: "api/orders#list-orders"
title: "Orders API"
heading: "List orders"
group: "API"
order: 47
url: "/docs/api/orders#list-orders"
anchor: "list-orders"
terms: ["list","orders","lists","newest","start","first","queued","ones","queue","order","filterable","host","repo","state","orchestrator","requester","more","curl","https","hevmind","running","needs","review","authorization","bearer","requested","local","issue","since","ordersummary","kind","position","branch","head","outcome","session","traces","sessions","hevapikey","come"]
hash: "4d421bc410fbe639772f55ccada82b260417fa03875c90d6d1ed75424ec9e38f"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"https://kit.hevmind.com/v1/orders?state=running\u0026state=needs-review\u0026repo=kit\" \\\n  -H \"Authorization: Bearer $HEV_API_KEY\"","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"start","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"host","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"repo","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"state","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"orchestrator","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"o-","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"requested_by","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"local:hev","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"issue","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"FAC-12","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"since","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"OrderSummary","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"id","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"kind","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"run","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"position","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"branch","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"head_sha","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"pr_url","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"outcome","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"end","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"session_id","chunkId":"api/orders#list-orders"},{"kind":"code","literal":"/v1/traces/sessions/{id}","chunkId":"api/orders#list-orders"}]
sources: [{"chunkId":"api/orders#list-orders","url":"/docs/api/orders#list-orders","anchor":"list-orders"}]
---

Lists orders newest start first with queued ones first in queue order, filterable by host, repo, state, orchestrator, requester and more.
