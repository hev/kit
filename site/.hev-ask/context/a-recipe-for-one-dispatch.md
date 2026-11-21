---
id: "context#a-recipe-for-one-dispatch"
title: "Composable context"
heading: "A recipe for one dispatch"
group: "Use"
order: 174
url: "/docs/context#a-recipe-for-one-dispatch"
anchor: "a-recipe-for-one-dispatch"
terms: ["recipe","dispatch","file","passed","amend","instead","committing","login","redirect","context","narrowed","json","show","order","carried","files","written","command","line","change","already","sent","must","under","does","need","also","name","verify","tell","digest","docs","https","hevkit","pages","llms","checks","adds","contextrecipe","base64"]
hash: "d585b053c489dd7cfaf88d09b0c8e116ee14834b8929c38f1b10138675820b79"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch fix-login-redirect.md --context-file narrowed.json\nhev dispatch run web 'Fix the login redirect' --context-file narrowed.json\nhev dispatch amend fix-login-redirect fix-login-redirect.md --context-file narrowed.json","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"--context-file","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context_recipe:","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context_recipe","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context-recipe.json","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"hev dispatch show","chunkId":"context#a-recipe-for-one-dispatch"}]
sources: [{"chunkId":"context#a-recipe-for-one-dispatch","url":"/docs/context#a-recipe-for-one-dispatch","anchor":"a-recipe-for-one-dispatch"}]
---

A one-off recipe file can be passed with dispatch, run or amend instead of committing one.
