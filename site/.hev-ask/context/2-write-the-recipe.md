---
id: "context#2-write-the-recipe"
title: "Composable context"
heading: "2. Write the recipe"
group: "Use"
order: 127
url: "/docs/context#2-write-the-recipe"
anchor: "2-write-the-recipe"
terms: ["write","recipe","step","copy","improved","repository","down","kind","work","needs","removing","unneeded","sources","narrowing","rest","recipes","meant","small","specific","kept","repo","reviewed","like","code","note","file","does","name","taken","worker","worktree","while","runs","mkdir","context","project","assessment","json","docs","editor"]
hash: "acebdedcb009398704d119f8eae3a077ddb8236411e54025a67d0b27b8a2778b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"mkdir -p .hev/context\ncp /tmp/my-project-assessment/improved.recipe.json .hev/context/docs.json\n$EDITOR .hev/context/docs.json","chunkId":"context#2-write-the-recipe"},{"kind":"code","literal":"In the repository root, create .hev/context and copy /tmp/my-project-assessment/improved.recipe.json to .hev/context/docs.json. Then cut its sources down to what docs changes need, and show me the sources you kept and dropped before saving. Keep a skill's files at their original targets.\nDocs: https://hevkit.com/docs/context#2-write-the-recipe (all pages: https://hevkit.com/llms.txt)","chunkId":"context#2-write-the-recipe"}]
sources: [{"chunkId":"context#2-write-the-recipe","url":"/docs/context#2-write-the-recipe","anchor":"2-write-the-recipe"}]
---

Step two: copy the improved recipe into the repository and cut it down to what one kind of work needs, removing unneeded sources and narrowing the rest. Recipes are meant to be small and specific and kept in the repo so they are reviewed like code; note that a file the recipe does not name is taken out of the worker's worktree while it runs.
