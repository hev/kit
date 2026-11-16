---
id: "windows#credentials"
title: "Windows"
heading: "Credentials"
group: "Reference"
order: 347
url: "/docs/windows#credentials"
anchor: "credentials"
terms: ["credentials","windows","rely","environment","forwarding","reach","harness","fetch","inside","wrapper","keep","arguments","example","reading","1password","azure","vault","through","daemon","cache","must","pass","unchanged","wait","return","exit","code","powershell","argument","list","unsupported","node","directly","shim","javascript","entry","point","instead","batch","file"]
hash: "bd6fb3c0eb7266d674f84e6d6179c3ebe442f727a1ebaf29c85bfbe8997c1961"
mode: "source-primary"
facts: [{"kind":"code","literal":"harness_wrapper","chunkId":"windows#credentials"},{"kind":"code","literal":"hev secrets read","chunkId":"windows#credentials"},{"kind":"code","literal":"--","chunkId":"windows#credentials"},{"kind":"code","literal":"ProcessStartInfo.ArgumentList","chunkId":"windows#credentials"}]
sources: [{"chunkId":"windows#credentials","url":"/docs/windows#credentials","anchor":"credentials"}]
---

On Windows, do not rely on environment forwarding to reach the harness; fetch credentials inside the harness wrapper and keep them out of its arguments, for example by reading 1Password or Azure Key Vault through the daemon's cache. The wrapper must pass all arguments through unchanged, wait, and return the harness's exit code, using PowerShell 7 with an argument list (Windows PowerShell 5 is unsupported), and run node directly on an npm shim's JavaScript entry point instead of a batch file.
