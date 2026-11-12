---
id: "windows#credentials"
title: "Windows"
heading: "Credentials"
group: "Reference"
order: 316
url: "/docs/windows#credentials"
anchor: "credentials"
terms: ["credentials","rely","environment","forwarding","reach","harness","windows","fetch","inside","wrapper","example","vault","keeping","arguments","receives","separator","name","must","pass","unchanged","wait","exit","code","powershell","argument","list","unsupported","script","given","processstartinfo","argumentlist","harnesswrapper","keep","gets","every","through","reparses","native","supported","creds"]
hash: "0595edadb7bdd24858e71c82eaf4ac0a57ccfe1b05870397ca72076a3bd2fc02"
mode: "source-primary"
facts: [{"kind":"code","literal":"harness_wrapper","chunkId":"windows#credentials"},{"kind":"code","literal":"--","chunkId":"windows#credentials"},{"kind":"code","literal":"ProcessStartInfo.ArgumentList","chunkId":"windows#credentials"}]
sources: [{"chunkId":"windows#credentials","url":"/docs/windows#credentials","anchor":"credentials"}]
---

Do not rely on environment forwarding to reach the harness on Windows; fetch credentials inside a harness wrapper, for example from a vault, keeping them out of its arguments. The wrapper receives a separator, the harness name and its arguments, must pass them unchanged, wait and exit with the harness's code. Use PowerShell 7 with an argument list; Windows PowerShell 5 is unsupported. An example script is given.
