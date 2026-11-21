---
id: "windows#credentials"
title: "Windows"
heading: "Credentials"
group: "Reference"
order: 461
url: "/docs/windows#credentials"
anchor: "credentials"
terms: ["credentials","windows","rely","environment","forwarding","harness","fetch","inside","wrapper","must","pass","arguments","through","unchanged","return","exit","code","powershell","rather","secrets","read","processstartinfo","argumentlist","reach","harnesswrapper","example","vault","keep","reads","1password","azure","daemon","cache","without","putting","value","command","line","host","only"]
hash: "bd6fb3c0eb7266d674f84e6d6179c3ebe442f727a1ebaf29c85bfbe8997c1961"
mode: "source-primary"
facts: [{"kind":"code","literal":"harness_wrapper","chunkId":"windows#credentials"},{"kind":"code","literal":"hev secrets read","chunkId":"windows#credentials"},{"kind":"code","literal":"--","chunkId":"windows#credentials"},{"kind":"code","literal":"ProcessStartInfo.ArgumentList","chunkId":"windows#credentials"}]
sources: [{"chunkId":"windows#credentials","url":"/docs/windows#credentials","anchor":"credentials"}]
---

On Windows don't rely on environment forwarding to the harness; fetch credentials inside the harness wrapper, which must pass arguments through unchanged and return the harness exit code, using PowerShell 7 rather than Windows PowerShell 5.
