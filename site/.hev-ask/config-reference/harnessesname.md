---
id: "config-reference#harnessesname"
title: "Config reference"
heading: "[harnesses.NAME]"
group: "Reference"
order: 158
url: "/docs/config-reference#harnessesname"
anchor: "harnessesname"
terms: ["harnesses","name","optional","harness","launch","settings","target","config","codex","claude","applying","dispatch","runs","validators","orchestrator","rounds","model","provider","gemini","refs","example","credential","modelprovider","azkv","secret","meta","json","apply","every","host","field","type","default","read","meaning","string","none","only","passed","without"]
hash: "574789f9931c6b0046a5397dbc0a931ceb45e6bff23e06bf927ad68eb24aeb4c"
mode: "source-primary"
facts: [{"kind":"code","literal":"[harnesses.agy]\nmodel_provider = \"gemini\"\n\n[harnesses.agy.env_refs]\nGEMINI_API_KEY = \"op://example/gemini-key/credential\"","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"[harnesses.NAME]","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"codex","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"claude","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"agy","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"model_provider","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"--settings '{\"modelProvider\":\"...\"}'","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"gemini","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"GEMINI_API_KEY","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"env_refs","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"op://","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"azkv://","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"secret://","chunkId":"config-reference#harnessesname"},{"kind":"code","literal":"meta.json","chunkId":"config-reference#harnessesname"}]
sources: [{"chunkId":"config-reference#harnessesname","url":"/docs/config-reference#harnessesname","anchor":"harnessesname"}]
---

Optional per-harness launch settings in the target's config for codex, claude or agy, applying to dispatch runs, validators and orchestrator rounds.
