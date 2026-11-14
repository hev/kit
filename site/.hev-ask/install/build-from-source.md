---
id: "install#build-from-source"
title: "Install"
heading: "Build from source"
group: "Start"
order: 180
url: "/docs/install#build-from-source"
anchor: "build-from-source"
terms: ["build","source","built","later","make","sure","resulting","binary","path","including","interactive","sessions","host","prefix","install","github","gopath","check","version","first","stop","older","lands","directory","session","sees","machine","dispatched","work","verify","docs","https","hevkit","pages","llms","puts","seen","pathprefix","dispatch","config"]
hash: "19525c8d5dd7e17bc24d20ba09cae91b9b8961f43ef7c954070787217f7b8761"
mode: "agent-primary"
facts: [{"kind":"code","literal":"go install github.com/hev/kit/cmd/hev@v0.6.2","chunkId":"install#build-from-source"},{"kind":"code","literal":"hev","chunkId":"install#build-from-source"},{"kind":"code","literal":"$(go env GOPATH)/bin","chunkId":"install#build-from-source"},{"kind":"code","literal":"path_prefix","chunkId":"install#build-from-source"},{"kind":"value","literal":"1.26","chunkId":"install#build-from-source"}]
sources: [{"chunkId":"install#build-from-source","url":"/docs/install#build-from-source","anchor":"build-from-source"}]
---

hev can be built from source with Go 1.26 or later; make sure the resulting binary is on the PATH, including the one non-interactive SSH sessions see, or use a host path prefix.
