---
id: "install#build-from-source"
title: "Install"
heading: "Build from source"
group: "Start"
order: 187
url: "/docs/install#build-from-source"
anchor: "build-from-source"
terms: ["build","source","binary","recent","toolchain","result","lands","directory","must","path","interactive","sessions","machine","dispatched","work","supplied","through","prefix","dispatch","config","install","github","gopath","later","check","version","first","stop","older","make","sure","including","session","sees","verify","docs","https","hevkit","pages","llms"]
hash: "19525c8d5dd7e17bc24d20ba09cae91b9b8961f43ef7c954070787217f7b8761"
mode: "agent-primary"
facts: [{"kind":"code","literal":"go install github.com/hev/kit/cmd/hev@v0.6.2","chunkId":"install#build-from-source"},{"kind":"code","literal":"hev","chunkId":"install#build-from-source"},{"kind":"code","literal":"$(go env GOPATH)/bin","chunkId":"install#build-from-source"},{"kind":"code","literal":"path_prefix","chunkId":"install#build-from-source"},{"kind":"value","literal":"1.26","chunkId":"install#build-from-source"}]
sources: [{"chunkId":"install#build-from-source","url":"/docs/install#build-from-source","anchor":"build-from-source"}]
---

You can build the binary from source with a recent Go toolchain. The result lands in Go's bin directory, which must be on the PATH that non-interactive SSH sessions see if the machine will run dispatched work, or be supplied through a path prefix in the dispatch config.
