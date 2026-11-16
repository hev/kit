---
id: "windows#credentials"
title: "Windows"
heading: "Credentials"
group: "Reference"
order: 411
url: "/docs/windows#credentials"
anchor: "credentials"
terms: ["credentials","windows","rely","environment","forwarding","harness","fetch","inside","wrapper","script","example","vault","keep","arguments","must","pass","every","argument","through","unchanged","wait","exit","code","requires","powershell","since","reparses","native","unsupported","page","gives","also","handles","batch","file","shims","running","node","shim","javascript"]
hash: "bd6fb3c0eb7266d674f84e6d6179c3ebe442f727a1ebaf29c85bfbe8997c1961"
mode: "source-primary"
facts: [{"kind":"code","literal":"harness_wrapper","chunkId":"windows#credentials"},{"kind":"code","literal":"hev secrets read","chunkId":"windows#credentials"},{"kind":"code","literal":"--","chunkId":"windows#credentials"},{"kind":"code","literal":"ProcessStartInfo.ArgumentList","chunkId":"windows#credentials"}]
sources: [{"chunkId":"windows#credentials","url":"/docs/windows#credentials","anchor":"credentials"}]
---

On Windows do not rely on environment forwarding to get credentials to the harness; fetch them inside a harness wrapper script, for example from a vault, and keep them out of arguments. The wrapper must pass every harness argument through unchanged, wait, and exit with the harness's exit code, which requires PowerShell 7, since Windows PowerShell 5 reparses native arguments and is unsupported. The page gives an example wrapper that also handles npm batch-file shims by running Node on the shim's JavaScript entry point directly.
