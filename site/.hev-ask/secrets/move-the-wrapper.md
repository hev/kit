---
id: "secrets#move-the-wrapper"
title: "Secrets"
heading: "Move the wrapper"
group: "Operate"
order: 317
url: "/docs/secrets#move-the-wrapper"
anchor: "move-the-wrapper"
terms: ["move","wrapper","shows","converting","windows","decrypted","token","fetched","secret","separately","keeps","boundary","checks","resolves","references","single","batch","through","cache","only","file","uses","clean","child","environment","explicit","allowlist","mode","direct","reads","happen","aborts","before","starting","reference","missing","came","protected","store","never"]
hash: "04f75fa60bd6d1d7eff293044179ba0a3ff55ef1d4c9821382c5eb82a3d11fa9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# Before: token came from the wrapper's protected store, never ambient fallback.\n$env:OP_SERVICE_ACCOUNT_TOKEN = $protectedToken\n$env:BUILD_TOKEN = \u0026 op.exe read 'op://example/build/token'\n$env:PACKAGE_TOKEN = \u0026 op.exe read 'op://example/package/token'\n$env:REPORT_TOKEN = \u0026 op.exe read 'op://example/report/token'\n\u0026 'C:\\Example\\bin\\worker.exe' @args","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"[secrets]\nmanager = \"1password\"\nvault = \"example\"\nfield = \"token\"","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"BUILD_TOKEN=secret://build\nPACKAGE_TOKEN=secret://package\nREPORT_TOKEN=secret://report","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"op","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"C:\\Example\\config\\worker.env","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"--clean-env","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"--pass-env NAME","chunkId":"secrets#move-the-wrapper"},{"kind":"code","literal":"--cache-only","chunkId":"secrets#move-the-wrapper"}]
sources: [{"chunkId":"secrets#move-the-wrapper","url":"/docs/secrets#move-the-wrapper","anchor":"move-the-wrapper"}]
---

Shows converting a Windows wrapper that decrypted its own token and fetched each secret separately into one that keeps its boundary checks but resolves all references in a single batch through the cache from a references-only env file. It uses a clean child environment with an explicit allowlist, cache-only mode so no direct reads happen, and aborts before starting the child if any reference is missing.
