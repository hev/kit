---
id: "windows#get-hevexe"
title: "Windows"
heading: "Get hev.exe"
group: "Reference"
order: 462
url: "/docs/windows#get-hevexe"
anchor: "get-hevexe"
terms: ["download","windows","amd64","release","verify","against","checksums","extract","permanent","directory","user","path","help","powershell","terminal","build","goos","goarch","userprofile","github","kit0","2windowsamd64","archive","such","open","capture","task","records","binary","absolute","dispatch","finds","through","keep","place","instead","later","checkout","source","machine"]
hash: "aad25311d5c9edb78bbeacbb6b2c035b2df3b985f7cdb08a8704d4d03672cf99"
mode: "source-primary"
facts: [{"kind":"code","literal":"go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"GOOS=windows GOARCH=amd64 go build -o hev.exe ./cmd/hev","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"checksums.txt","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"%USERPROFILE%\\bin","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"hev.exe --help","chunkId":"windows#get-hevexe"},{"kind":"code","literal":"v0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"github.com","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"0.6.2","chunkId":"windows#get-hevexe"},{"kind":"value","literal":"1.26","chunkId":"windows#get-hevexe"}]
sources: [{"chunkId":"windows#get-hevexe","url":"/docs/windows#get-hevexe","anchor":"get-hevexe"}]
---

Download the Windows amd64 zip from the 0.6.2 release, verify against checksums, extract hev.exe to a permanent directory on the user PATH, and run help in a new PowerShell terminal.
