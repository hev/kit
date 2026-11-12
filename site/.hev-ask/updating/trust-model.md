---
id: "updating#trust-model"
title: "Updating and channels"
heading: "Trust model"
group: "Operate"
order: 309
url: "/docs/updating#trust-model"
anchor: "trust-model"
terms: ["trust","model","update","fetches","fixed","binary","official","releases","https","flag","config","request","field","able","change","network","attacker","only","make","fail","since","digest","checks","block","substitution","owner","move","host","published","build","while","invited","member","cannot","supply","hook","attestation","verify","edge","linux"]
hash: "94318b0c968d73a5f7bf1ced254f9c2dbb9f5b0c0f1d94b6e24700dbae8a438d"
mode: "agent-primary"
facts: [{"kind":"code","literal":"gh attestation verify kit_X.Y.Z-edge.SHA_linux_amd64.tar.gz --repo hev/kit","chunkId":"updating#trust-model"},{"kind":"code","literal":"Check the build provenance of a kit release archive: run `gh attestation verify kit_X.Y.Z-edge.SHA_linux_amd64.tar.gz --repo hev/kit`, with the archive file I downloaded. It needs the GitHub CLI, logged in. Tell me whether it verified and which workflow and commit the attestation names.\nDocs: https://hevkit.com/docs/updating#trust-model (all pages: https://hevkit.com/llms.txt)","chunkId":"updating#trust-model"},{"kind":"code","literal":"hev update","chunkId":"updating#trust-model"},{"kind":"code","literal":"github.com/hev/kit","chunkId":"updating#trust-model"},{"kind":"code","literal":"api.github.com","chunkId":"updating#trust-model"},{"kind":"code","literal":"--to","chunkId":"updating#trust-model"},{"kind":"code","literal":"hev/kit","chunkId":"updating#trust-model"}]
sources: [{"chunkId":"updating#trust-model","url":"/docs/updating#trust-model","anchor":"trust-model"}]
---

What update fetches and from where is fixed in the binary: official hev/kit releases over HTTPS, with no flag, config or request field able to change it. A network attacker can only make an update fail, since TLS and digest checks block substitution. An owner can move a host to any published build, while an invited member cannot, and no one can supply a binary, URL, digest or hook.
