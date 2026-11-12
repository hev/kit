---
id: "windows#get-hevexe"
title: "Windows"
heading: "Get hev.exe"
group: "Reference"
order: 303
url: "/docs/windows#get-hevexe"
anchor: "get-hevexe"
terms: ["download","windows","amd64","release","verify","against","checksums","permanent","directory","user","path","since","capture","task","records","absolute","dispatch","finds","through","also","build","source","cross","compile","unix","goos","goarch","userprofile","help","github","kit0","2windowsamd64","archive","extract","such","open","powershell","terminal","binary","keep"]
hash: "aad25311d5c9edb78bbeacbb6b2c035b2df3b985f7cdb08a8704d4d03672cf99"
mode: "source-primary"
facts: [{"kind":"code","literal":"go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"GOOS=windows GOARCH=amd64 go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"checksums.txt","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"%USERPROFILE%\\bin","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe --help","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"v0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"github.com","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"1.26","chunkId":"windows#get-hevexe"}]
sources: [{"chunkId":"windows#get-hevexe","url":"/docs/windows#get-hevexe","anchor":"get-hevexe"}]
---

Download the Windows amd64 zip from the release, verify it against the checksums, and put hev.exe in a permanent directory on your user PATH, since the capture task records its absolute path and dispatch finds it through PATH over SSH. You can also build it from source with Go on Windows or cross-compile from Unix.
