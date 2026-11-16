---
id: "context#a-recipe-for-one-dispatch"
title: "Composable context"
heading: "A recipe for one dispatch"
group: "Use"
order: 133
url: "/docs/context#a-recipe-for-one-dispatch"
anchor: "a-recipe-for-one-dispatch"
terms: ["recipe","dispatch","carried","inside","order","single","inline","amend","without","committing","travels","through","queue","amends","host","moves","either","names","committed","carries","both","pins","record","first","build","later","attempts","read","pinned","copy","worker","cannot","change","login","redirect","context","file","narrowed","json","show"]
hash: "d585b053c489dd7cfaf88d09b0c8e116ee14834b8929c38f1b10138675820b79"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch fix-login-redirect.md --context-file narrowed.json\nhev dispatch run web 'Fix the login redirect' --context-file narrowed.json\nhev dispatch amend fix-login-redirect fix-login-redirect.md --context-file narrowed.json","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"--context-file","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context_recipe:","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context_recipe","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"context-recipe.json","chunkId":"context#a-recipe-for-one-dispatch"},{"kind":"code","literal":"hev dispatch show","chunkId":"context#a-recipe-for-one-dispatch"}]
sources: [{"chunkId":"context#a-recipe-for-one-dispatch","url":"/docs/context#a-recipe-for-one-dispatch","anchor":"a-recipe-for-one-dispatch"}]
---

A recipe can be carried inside an order for a single dispatch, an inline order or an amend, without committing it; it travels with the order through the queue, amends and host moves, up to 1 MiB. An order either names a committed recipe or carries one, not both. The host pins the carried recipe in the run record at the first build so later attempts read the pinned copy and a worker cannot change it.
