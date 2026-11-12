---
id: "windows#credentials"
title: "Windows"
heading: "Credentials"
group: "Reference"
order: 302
url: "/docs/windows#credentials"
anchor: "credentials"
terms: ["credentials","windows","rely","environment","forwarding","reach","harness","fetch","inside","wrapper","script","receives","separator","name","arguments","passes","through","unchanged","waits","returns","exit","code","powershell","argument","list","reparses","unsupported","batch","shims","node","javascript","entry","point","instead","processstartinfo","argumentlist","harnesswrapper","example","vault","keep"]
hash: "0595edadb7bdd24858e71c82eaf4ac0a57ccfe1b05870397ca72076a3bd2fc02"
mode: "source-primary"
facts: [{"kind":"code","literal":"harness_wrapper","chunkId":"windows#credentials"},{"kind":"code","literal":"--","chunkId":"windows#credentials"},{"kind":"code","literal":"ProcessStartInfo.ArgumentList","chunkId":"windows#credentials"}]
sources: [{"chunkId":"windows#credentials","url":"/docs/windows#credentials","anchor":"credentials"}]
---

On Windows do not rely on environment forwarding to reach the harness; fetch credentials inside a harness wrapper script that receives a separator, the harness name and its arguments, passes them through unchanged, waits and returns the harness exit code. Use PowerShell 7 with an argument list (Windows PowerShell 5 reparses arguments and is unsupported), and for npm batch shims run node on the JavaScript entry point instead.
