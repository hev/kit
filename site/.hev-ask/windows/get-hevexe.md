---
id: "windows#get-hevexe"
title: "Windows"
heading: "Get hev.exe"
group: "Reference"
order: 365
url: "/docs/windows#get-hevexe"
anchor: "get-hevexe"
terms: ["windows","executable","release","verify","checksum","place","permanent","directory","user","path","confirm","help","runs","terminal","capture","task","records","absolute","dispatch","finds","alternatively","build","recent","toolchain","cross","compile","unix","goos","goarch","amd64","checksums","userprofile","github","download","kit0","2windowsamd64","archive","against","extract","such"]
hash: "aad25311d5c9edb78bbeacbb6b2c035b2df3b985f7cdb08a8704d4d03672cf99"
mode: "source-primary"
facts: [{"kind":"code","literal":"go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"GOOS=windows GOARCH=amd64 go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"checksums.txt","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"%USERPROFILE%\\bin","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe --help","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"v0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"github.com","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"1.26","chunkId":"windows#get-hevexe"}]
sources: [{"chunkId":"windows#get-hevexe","url":"/docs/windows#get-hevexe","anchor":"get-hevexe"}]
---

Get the Windows executable from the release zip, verify its checksum, place it in a permanent directory on your user PATH and confirm help runs in a new terminal; the capture task records its absolute path and dispatch finds it via PATH over SSH. Alternatively build it with a recent Go toolchain on Windows or cross-compile from Unix.
