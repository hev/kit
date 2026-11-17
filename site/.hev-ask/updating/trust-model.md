---
id: "updating#trust-model"
title: "Updating and channels"
heading: "Trust model"
group: "Operate"
order: 402
url: "/docs/updating#trust-model"
anchor: "trust-model"
terms: ["trust","model","updater","fetches","fixed","binary","official","repository","releases","https","nothing","able","redirect","network","attacker","only","block","download","since","digest","check","stop","substitution","owner","dispatch","access","move","host","published","build","cannot","supply","hook","invited","member","whoever","controls","publish","every","channel"]
hash: "94318b0c968d73a5f7bf1ced254f9c2dbb9f5b0c0f1d94b6e24700dbae8a438d"
mode: "agent-primary"
facts: [{"kind":"code","literal":"gh attestation verify kit_X.Y.Z-edge.SHA_linux_amd64.tar.gz --repo hev/kit","chunkId":"updating#trust-model"},{"kind":"code","literal":"Check the build provenance of a kit release archive: run `gh attestation verify kit_X.Y.Z-edge.SHA_linux_amd64.tar.gz --repo hev/kit`, with the archive file I downloaded. It needs the GitHub CLI, logged in. Tell me whether it verified and which workflow and commit the attestation names.\nDocs: https://hevkit.com/docs/updating#trust-model (all pages: https://hevkit.com/llms.txt)","chunkId":"updating#trust-model"},{"kind":"code","literal":"hev update","chunkId":"updating#trust-model"},{"kind":"code","literal":"github.com/hev/kit","chunkId":"updating#trust-model"},{"kind":"code","literal":"api.github.com","chunkId":"updating#trust-model"},{"kind":"code","literal":"--to","chunkId":"updating#trust-model"},{"kind":"code","literal":"hev/kit","chunkId":"updating#trust-model"}]
sources: [{"chunkId":"updating#trust-model","url":"/docs/updating#trust-model","anchor":"trust-model"}]
---

What the updater fetches and from where is fixed in the binary: the official repository's releases over HTTPS, with nothing able to redirect it. A network attacker can only block the download, since TLS and the digest check stop substitution; an owner with dispatch access can move a host to any published build but cannot supply a binary, URL, digest or hook, and an invited member can do nothing; whoever controls the repository's releases can publish a build every host on that channel installs, the same trust as installing a release. The digest proves consistency and an intact download, not who built it; edge archives carry a signed build provenance attestation you can check by hand, which the updater itself does not verify.
