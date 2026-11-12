---
id: "context#4-commit-the-recipe"
title: "Composable context"
heading: "4. Commit the recipe"
group: "Use"
order: 73
url: "/docs/context#4-commit-the-recipe"
anchor: "4-commit-the-recipe"
terms: ["commit","recipe","push","because","host","reads","repository","base","origin","worker","cannot","change","next","attempt","gets","context","docs","json","changes","main","message","since","hosts","read","protected","another","branch","stop","instead","pushing","verify","show","https","hevkit","pages","llms","runs","work","pushed"]
hash: "1d96868d42211e7d25c1678b51dcc9b8b16cf7e0c323573455fdb17c452b9050"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git add .hev/context/docs.json\ngit commit -m \"Add a context recipe for docs changes\"\ngit push origin main","chunkId":"context#4-commit-the-recipe"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"context#4-commit-the-recipe"}]
sources: [{"chunkId":"context#4-commit-the-recipe","url":"/docs/context#4-commit-the-recipe","anchor":"4-commit-the-recipe"}]
---

Commit and push the recipe because the host reads it from the repository at the run's base commit on origin; a worker cannot change the recipe its own next attempt gets.
