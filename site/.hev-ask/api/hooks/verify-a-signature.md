---
id: "api/hooks#verify-a-signature"
title: "Hooks API"
heading: "Verify a signature"
group: "API"
order: 41
url: "/docs/api/hooks#verify-a-signature"
anchor: "verify-a-signature"
terms: ["verify","signature","deliveries","computing","hmac","sha256","timestamp","body","hook","secret","comparing","constant","time","header","during","rotation","rejecting","stale","timestamps","yaml","compute","value","request","compare","carries","separated","comma","first","either","match","reject","more","five","minutes","headers","dict","bytes","bool","return","false"]
hash: "f0372132896540a4d5f65c33cbdb14770f505e3700eccd4a39be73e27c7acc1a"
mode: "source-primary"
facts: [{"kind":"code","literal":"X-Hev-Timestamp","chunkId":"api/hooks#verify-a-signature"},{"kind":"code","literal":"sha256=","chunkId":"api/hooks#verify-a-signature"},{"kind":"code","literal":"X-Hev-Signature-256","chunkId":"api/hooks#verify-a-signature"},{"kind":"code","literal":"X-Hub-Signature-256","chunkId":"api/hooks#verify-a-signature"},{"kind":"value","literal":"kit-v1.yaml","chunkId":"api/hooks#verify-a-signature"}]
sources: [{"chunkId":"api/hooks#verify-a-signature","url":"/docs/api/hooks#verify-a-signature","anchor":"verify-a-signature"}]
---

Verify deliveries by computing an HMAC-SHA256 over the timestamp, a dot and the raw body using the hook's secret, comparing in constant time to each signature in the header (two during rotation), and rejecting stale timestamps.
