---
id: "cli#context"
title: "CLI"
heading: "Context"
group: "Reference"
order: 35
url: "/docs/cli#context"
anchor: "context"
terms: ["context","commands","composable","assess","project","instruction","skill","files","extract","sources","build","recipe","bundle","diff","bundles","review","loopback","page","every","output","directory","must","nothing","overwritten","root","assessment","json","improved","file","materialized","index","html","before","after","addr","name","attempt","command","does","find"]
hash: "b325fd84cf09fe864f1b03eb5b788b34cf0bc95db9c69d41d4452adc27182a16"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev context assess --root DIR --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"assessment.json","chunkId":"cli#context"},{"kind":"code","literal":"recipe.json","chunkId":"cli#context"},{"kind":"code","literal":"improved.recipe.json","chunkId":"cli#context"},{"kind":"code","literal":"hev context extract --root DIR --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"NEW/sources","chunkId":"cli#context"},{"kind":"code","literal":"hev context build --root DIR --recipe FILE --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"context.md","chunkId":"cli#context"},{"kind":"code","literal":"materialized/","chunkId":"cli#context"},{"kind":"code","literal":"bundle.json","chunkId":"cli#context"},{"kind":"code","literal":"index.html","chunkId":"cli#context"},{"kind":"code","literal":"hev context diff --before A --after B --out NEW","chunkId":"cli#context"},{"kind":"code","literal":"hev context review --bundle DIR","chunkId":"cli#context"},{"kind":"code","literal":"--addr","chunkId":"cli#context"},{"kind":"code","literal":"hev context review --run NAME","chunkId":"cli#context"},{"kind":"code","literal":"--attempt N","chunkId":"cli#context"},{"kind":"code","literal":"--out","chunkId":"cli#context"}]
sources: [{"chunkId":"cli#context","url":"/docs/cli#context","anchor":"context"}]
---

Commands for composable context: assess a project's instruction and skill files, extract sources, build a recipe into a bundle, diff two bundles, and review a bundle or a run's bundle on a loopback web page. Every output directory must be new and nothing is overwritten.
