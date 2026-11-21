---
id: "install#build-from-source"
title: "Install"
heading: "Build from source"
group: "Start"
order: 250
url: "/docs/install#build-from-source"
anchor: "build-from-source"
terms: ["build","source","later","install","make","sure","directory","path","including","seen","interactive","machine","runs","dispatched","work","pathprefix","github","gopath","prefix","check","version","first","stop","older","binary","lands","session","sees","verify","docs","https","hevkit","pages","llms","puts","dispatch","config","clustering"]
hash: "f0672899ea743fa785c0786dce9be89b6522423acbabf4a86ddde45ee0c68cc2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"go install github.com/hev/kit/cmd/hev@v0.6.2","chunkId":"install#build-from-source"},{"kind":"code","literal":"hev","chunkId":"install#build-from-source"},{"kind":"code","literal":"$(go env GOPATH)/bin","chunkId":"install#build-from-source"},{"kind":"code","literal":"path_prefix","chunkId":"install#build-from-source"},{"kind":"value","literal":"1.26","chunkId":"install#build-from-source"}]
sources: [{"chunkId":"install#build-from-source","url":"/docs/install#build-from-source","anchor":"build-from-source"}]
---

Build from source with Go 1.26 or later via go install; make sure the Go bin directory is on PATH, including the PATH seen by non-interactive SSH if the machine runs dispatched work (or use pathprefix).
