---
name: hev-context
description: Review and narrow a dispatch's instruction context with the person before sending it. Use before every hev dispatch you send for a person to a repository that has AGENTS.md, CLAUDE.md, agents or skills.
metadata:
  audience: owner
---

Repositories' instruction files only grow. A worker sent to fix one thing should
get the slice of them it needs, not all of it. Before you dispatch an order a
person asked for, do this short review with them. It changes only what that one
worker sees, in its own worktree, for that run. It never changes the repository's
files, the person's checkout or anything committed.

1. **Plan.** With the order written, run it against a checkout of the order's
   repository at its base:

   ```sh
   hev context plan order.md --root REPO --out "$(mktemp -d)/plan"
   ```

   It lists every instruction file, agent and skill with its bytes, its sections
   (`## Heading`, with sizes) and the assessment's findings, and writes
   `recipe.json` (everything, whole), `plan.json` and `index.html`. It calls no
   model. If it says there is nothing to narrow, dispatch as usual and skip the
   rest.

2. **Propose.** Read the order's intent and the files. Copy `recipe.json` and cut
   what this task does not need:
   - drop sections with `"drop": ["## Heading"]`, or keep only some with
     `"keep": [...]` (write headings exactly as `plan` prints them);
   - remove a source to leave that file out of the worker's worktree entirely.
     Files the recipe does not name are excluded, so cut only what you mean to;
     a skill's other files follow its `SKILL.md`;
   - keep whole any instruction file the order asks the worker to edit;
   - give one short reason for each cut.

   Check it builds and get the sizes:
   `hev context build --root REPO --recipe narrowed.json --out "$(mktemp -d)/b"`.
   A selector that matches nothing is an error. Fix it, don't drop it silently.

3. **Show the person one screen** and recommend. Don't hand them a
   questionnaire. Show what is kept, what is dropped and why, bytes before and
   after, and the review page (`index.html` in the plan output), then ask for a
   yes or an edit:

   ```text
   Context for fix-login-redirect (web, base main): 61.2 KB → 9.8 KB
   Keep  AGENTS.md  ## Setup, ## Testing, ## Auth   7.1 of 41.9 KB
         CLAUDE.md  whole                           1.9 KB
         skill testing                              0.8 KB
   Drop  AGENTS.md ## Release process   release work, not this task
         AGENTS.md ## Billing           billing code is untouched
         skill deploy                   the worker does not deploy
   Remark: ## Old CI repeats ## Testing with opposite retry advice.
   Review page: /tmp/tmp.x1/plan/index.html
   Dispatch with this? (yes, or tell me what to keep)
   ```

   Apply edits, and show the screen again only if the change is material.

4. **Dispatch with the agreed recipe:**
   `hev dispatch order.md --context-file narrowed.json`. Then
   `hev dispatch show NAME` prints the context digest and the form each
   instruction file reached the worker in. Tell the person the digest, so they
   can find the run by it later (`hev traces ls --context DIGEST`).

If the review shows sections that look stale, duplicated, contradictory or never
needed, you may say so in one line, as a remark. Don't offer or open a change to
the source file as part of a dispatch, and never edit a repository's instruction
files here: each dispatch takes only the slice it needs, and the files stay as
they are.

**Skip.** If the person says to skip the review, dispatch the order without
`--context-file`. The worker loads the repository's files whole, as before. A
recipe committed in the repository (`context: PATH`) also works without this
review.
