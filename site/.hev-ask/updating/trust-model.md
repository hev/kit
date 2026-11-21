---
id: "updating#trust-model"
title: "Updating and channels"
heading: "Trust model"
group: "Operate"
order: 447
url: "/docs/updating#trust-model"
anchor: "trust-model"
terms: ["trust","model","update","fetches","fixed","binary","only","github","releases","https","flag","config","request","change","attestation","verify","edge","linux","amd64","repo","check","build","provenance","release","archive","file","downloaded","needs","logged","tell","whether","verified","workflow","commit","names","docs","hevkit","updating","pages","llms"]
hash: "94318b0c968d73a5f7bf1ced254f9c2dbb9f5b0c0f1d94b6e24700dbae8a438d"
mode: "agent-primary"
facts: [{"kind":"code","literal":"gh attestation verify kit_X.Y.Z-edge.SHA_linux_amd64.tar.gz --repo hev/kit","chunkId":"updating#trust-model"},{"kind":"code","literal":"Check the build provenance of a kit release archive: run `gh attestation verify kit_X.Y.Z-edge.SHA_linux_amd64.tar.gz --repo hev/kit`, with the archive file I downloaded. It needs the GitHub CLI, logged in. Tell me whether it verified and which workflow and commit the attestation names.\nDocs: https://hevkit.com/docs/updating#trust-model (all pages: https://hevkit.com/llms.txt)","chunkId":"updating#trust-model"},{"kind":"code","literal":"hev update","chunkId":"updating#trust-model"},{"kind":"code","literal":"github.com/hev/kit","chunkId":"updating#trust-model"},{"kind":"code","literal":"api.github.com","chunkId":"updating#trust-model"},{"kind":"code","literal":"--to","chunkId":"updating#trust-model"},{"kind":"code","literal":"hev/kit","chunkId":"updating#trust-model"}]
sources: [{"chunkId":"updating#trust-model","url":"/docs/updating#trust-model","anchor":"trust-model"}]
---

What update fetches and from where is fixed in the binary: only github.com/hev/kit releases over HTTPS; no flag, config or request can change it.
