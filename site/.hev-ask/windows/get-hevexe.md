---
id: "windows#get-hevexe"
title: "Windows"
heading: "Get hev.exe"
group: "Reference"
order: 416
url: "/docs/windows#get-hevexe"
anchor: "get-hevexe"
terms: ["install","windows","download","archive","release","verify","against","checksums","extract","executable","permanent","directory","user","path","must","stay","because","capture","task","records","absolute","dispatch","finds","through","alternatively","build","source","recent","toolchain","cross","compile","unix","machine","goos","goarch","amd64","userprofile","help","github","kit0"]
hash: "aad25311d5c9edb78bbeacbb6b2c035b2df3b985f7cdb08a8704d4d03672cf99"
mode: "source-primary"
facts: [{"kind":"code","literal":"go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"GOOS=windows GOARCH=amd64 go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"checksums.txt","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"%USERPROFILE%\\bin","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe --help","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"v0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"github.com","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"1.26","chunkId":"windows#get-hevexe"}]
sources: [{"chunkId":"windows#get-hevexe","url":"/docs/windows#get-hevexe","anchor":"get-hevexe"}]
---

To install on Windows, download the Windows archive from the release, verify it against the checksums, and extract the executable into a permanent directory on your user path; it must stay put because the capture task records its absolute path and dispatch finds it through the path over SSH. Alternatively build it from source on Windows with a recent Go toolchain at the release tag, or cross-compile it from a Unix machine.
