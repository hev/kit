---
id: "context#2-write-the-recipe"
title: "Composable context"
heading: "2. Write the recipe"
group: "Use"
order: 126
url: "/docs/context#2-write-the-recipe"
anchor: "2-write-the-recipe"
terms: ["write","recipe","copy","improved","repository","trim","specific","kind","work","needs","keep","recipes","small","task","store","repo","reviewed","like","code","mkdir","context","project","assessment","json","docs","editor","root","create","sources","down","changes","need","show","kept","dropped","before","saving","skill","files","their"]
hash: "7ad25e3d0e9e0384dc537da6601bfca44e027c725cdf8af3f9a15cd18a83aa40"
mode: "agent-primary"
facts: [{"kind":"code","literal":"mkdir -p .hev/context\ncp /tmp/my-project-assessment/improved.recipe.json .hev/context/docs.json\n$EDITOR .hev/context/docs.json","chunkId":"context#2-write-the-recipe"},{"kind":"code","literal":"In the repository root, create .hev/context and copy /tmp/my-project-assessment/improved.recipe.json to .hev/context/docs.json. Then cut its sources down to what docs changes need, and show me the sources you kept and dropped before saving. Keep a skill's files at their original targets.\nDocs: https://hevkit.com/docs/context#2-write-the-recipe (all pages: https://hevkit.com/llms.txt)","chunkId":"context#2-write-the-recipe"}]
sources: [{"chunkId":"context#2-write-the-recipe","url":"/docs/context#2-write-the-recipe","anchor":"2-write-the-recipe"}]
---

Copy the improved recipe into the repository and trim it to what a specific kind of work needs; keep recipes small and task-specific and store them in the repo so they are reviewed like code.
