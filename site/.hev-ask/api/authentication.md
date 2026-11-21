---
id: "api#authentication"
title: "API overview"
heading: "Authentication"
group: "API"
order: 10
url: "/docs/api#authentication"
anchor: "authentication"
terms: ["authentication","every","route","takes","bearer","same","login","saves","environment","variable","resolves","account","namespaces","server","side","whoami","returns","caller","curl","https","hevmind","authorization","name","example","hevapikey","request","ever","names","namespace","store","tenant","read","only","gets","writes","clients","never","follow","redirect","attached"]
hash: "ba77a7ee0e3fdafc54486a389126d4cea489490648f62069334cbd39958c8790"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/whoami \\\n  -H \"Authorization: Bearer $HEV_API_KEY\"","chunkId":"api#authentication"},{"kind":"code","literal":"{ \"id\": \"ada\", \"name\": \"Ada Example\" }","chunkId":"api#authentication"},{"kind":"code","literal":"hk_…","chunkId":"api#authentication"},{"kind":"code","literal":"hev login","chunkId":"api#authentication"},{"kind":"code","literal":"[kit] key","chunkId":"api#authentication"},{"kind":"code","literal":"HEV_API_KEY","chunkId":"api#authentication"},{"kind":"code","literal":"403","chunkId":"api#authentication"}]
sources: [{"chunkId":"api#authentication","url":"/docs/api#authentication","anchor":"authentication"}]
---

Every route takes a bearer key, the same kit key that login saves or the API key environment variable; it resolves the account and namespaces server-side. A whoami route returns the caller.
