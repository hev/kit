---
id: "windows#credentials"
title: "Windows"
heading: "Credentials"
group: "Reference"
order: 364
url: "/docs/windows#credentials"
anchor: "credentials"
terms: ["credentials","windows","rely","environment","forwarding","give","harness","fetch","inside","wrapper","script","passes","every","argument","through","unchanged","waits","returns","exit","code","powershell","reparses","arguments","batch","shims","node","javascript","entry","point","instead","processstartinfo","argumentlist","reach","harnesswrapper","example","vault","keep","gets","name","must"]
hash: "0595edadb7bdd24858e71c82eaf4ac0a57ccfe1b05870397ca72076a3bd2fc02"
mode: "source-primary"
facts: [{"kind":"code","literal":"harness_wrapper","chunkId":"windows#credentials"},{"kind":"code","literal":"--","chunkId":"windows#credentials"},{"kind":"code","literal":"ProcessStartInfo.ArgumentList","chunkId":"windows#credentials"}]
sources: [{"chunkId":"windows#credentials","url":"/docs/windows#credentials","anchor":"credentials"}]
---

On Windows, don't rely on environment forwarding to give the harness credentials; fetch them inside a harness wrapper script that passes every argument through unchanged, waits, and returns the harness's exit code. Use PowerShell 7 (not 5, which reparses arguments), and for npm batch shims run node on the JavaScript entry point instead.
