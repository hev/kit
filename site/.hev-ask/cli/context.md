---
id: "cli#context"
title: "CLI"
heading: "Context"
group: "Reference"
order: 92
url: "/docs/cli#context"
anchor: "context"
terms: ["context","command","group","plan","recipe","order","assess","project","instruction","skill","files","extract","standalone","copy","build","bundle","diff","bundles","review","dispatch","given","local","page","every","output","directory","must","nothing","overwritten","root","repo","json","index","html","assessment","improved","sources","file","materialized","before"]
hash: "13fde93ce7807bf324fa770402f14534402f565748bcbebb2241bfad4fe6bd57"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev context plan ORDER.md --root REPO --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"recipe.json","chunkId":"cli#context"},{"kind":"code","literal":"plan.json","chunkId":"cli#context"},{"kind":"code","literal":"index.html","chunkId":"cli#context"},{"kind":"code","literal":"hev context assess --root DIR --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"assessment.json","chunkId":"cli#context"},{"kind":"code","literal":"improved.recipe.json","chunkId":"cli#context"},{"kind":"code","literal":"hev context extract --root DIR --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"NEW/sources","chunkId":"cli#context"},{"kind":"code","literal":"hev context build --root DIR --recipe FILE --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"context.md","chunkId":"cli#context"},{"kind":"code","literal":"materialized/","chunkId":"cli#context"},{"kind":"code","literal":"bundle.json","chunkId":"cli#context"},{"kind":"code","literal":"hev context diff --before A --after B --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"hev context review --bundle DIR","chunkId":"cli#context"},{"kind":"code","literal":"--addr","chunkId":"cli#context"},{"kind":"code","literal":"hev context review --run NAME","chunkId":"cli#context"},{"kind":"code","literal":"--attempt N","chunkId":"cli#context"},{"kind":"code","literal":"--out","chunkId":"cli#context"}]
sources: [{"chunkId":"cli#context","url":"/docs/cli#context","anchor":"context"}]
---

The context command group: plan a recipe from an order, assess a project's instruction and skill files, extract them into a standalone copy, build a recipe into a bundle, diff two bundles, and review a bundle or the bundle a dispatch run was given in a local page. Every output directory must be new; nothing is overwritten.
