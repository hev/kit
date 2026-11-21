---
id: "api/hooks#register-a-webhook"
title: "Hooks API"
heading: "Register a webhook"
group: "API"
order: 39
url: "/docs/api/hooks#register-a-webhook"
anchor: "register-a-webhook"
terms: ["register","webhook","posting","description","event","list","optional","blocking","flag","timeout","idempotency","response","includes","signing","secret","curl","https","hevmind","hooks","authorization","bearer","content","type","application","json","gate","example","page","needs","audit","order","changes","events","tool","false","include","enabled","true","session","start"]
hash: "36a332067d3440a53fd82fe1cbfd78872cf0cca2e6a92b30ae6caa6f64839035"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/hooks \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -H \"Idempotency-Key: ci-gate-1\" \\\n  -d '{\n    \"url\": \"https://hooks.example.com/kit\",\n    \"description\": \"Page on needs-you; audit order changes\",\n    \"events\": [\"needs-you\", \"order-*\", \"pre-tool\"],\n    \"blocking\": false,\n    \"include_content\": false\n  }'","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"url","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"https","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"422","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"events","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"description","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"enabled","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"true","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"blocking","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"false","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"session-start","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"prompt-submit","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"pre-tool","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"post-tool","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"timeout_ms","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"include_content","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"secret","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"201","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"Hook","chunkId":"api/hooks#register-a-webhook"},{"kind":"code","literal":"secret_hint","chunkId":"api/hooks#register-a-webhook"}]
sources: [{"chunkId":"api/hooks#register-a-webhook","url":"/docs/api/hooks#register-a-webhook","anchor":"register-a-webhook"}]
---

Register a webhook by posting its URL, description, event list, optional blocking flag and timeout, with an idempotency key. The response includes the signing secret.
