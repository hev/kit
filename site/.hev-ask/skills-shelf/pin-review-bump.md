---
id: "skills-shelf#pin-review-bump"
title: "Skills shelf"
heading: "Pin, review, bump"
group: "Operate"
order: 399
url: "/docs/skills-shelf#pin-review-bump"
anchor: "pin-review-bump"
terms: ["review","bump","pins","never","move","themselves","commits","diff","since","pack","specific","commit","exists","skills","shelf","pstack","diffstat","patch","full","1a2b3c4d5e","only","watch","nothing","moves","itself","refuses","without","branch","name","rewrites","reinstalls","indexes","stay","installed","running","worker","keeps","copy","safe","loop"]
hash: "11d9fa566de2ea60eb05a7dffb03ba9e8ffefbaaed5f49d94824628babadb0de"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev skills shelf diff pstack                    # commits and diffstat since the pin\nhev skills shelf diff pstack --patch            # the full diff to review\nhev skills shelf bump pstack --to 1a2b3c4d5e    # only a commit, only one on the pack's ref","chunkId":"skills-shelf#pin-review-bump"},{"kind":"code","literal":"bump","chunkId":"skills-shelf#pin-review-bump"},{"kind":"code","literal":"--to","chunkId":"skills-shelf#pin-review-bump"},{"kind":"code","literal":"hev watch","chunkId":"skills-shelf#pin-review-bump"},{"kind":"code","literal":"hev skills shelf diff PACK --patch","chunkId":"skills-shelf#pin-review-bump"},{"kind":"code","literal":"hev skills shelf bump PACK --to SHA","chunkId":"skills-shelf#pin-review-bump"}]
sources: [{"chunkId":"skills-shelf#pin-review-bump","url":"/docs/skills-shelf#pin-review-bump","anchor":"pin-review-bump"}]
---

Pins never move by themselves: you review the commits and diff since the pin, then bump the pack to a specific commit that exists on the pack.
