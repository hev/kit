---
id: "install#build-from-source"
title: "Install"
heading: "Build from source"
group: "Start"
order: 125
url: "/docs/install#build-from-source"
anchor: "build-from-source"
terms: ["build","source","recent","toolchain","install","make","sure","directory","path","interactive","sees","prefix","github","gopath","later","check","version","first","stop","older","binary","lands","including","session","machine","dispatched","work","verify","docs","https","hevkit","pages","llms","puts","seen","pathprefix","dispatch","config","hosts"]
hash: "19525c8d5dd7e17bc24d20ba09cae91b9b8961f43ef7c954070787217f7b8761"
mode: "agent-primary"
facts: [{"kind":"code","literal":"go install github.com/hev/kit/cmd/hev@v0.6.2","chunkId":"install#build-from-source"},{"kind":"code","literal":"hev","chunkId":"install#build-from-source"},{"kind":"code","literal":"$(go env GOPATH)/bin","chunkId":"install#build-from-source"},{"kind":"code","literal":"path_prefix","chunkId":"install#build-from-source"},{"kind":"value","literal":"1.26","chunkId":"install#build-from-source"}]
sources: [{"chunkId":"install#build-from-source","url":"/docs/install#build-from-source","anchor":"build-from-source"}]
---

With a recent Go toolchain you can build and install hev from source; make sure the Go bin directory is on the PATH that non-interactive SSH sees, or use a PATH prefix.
