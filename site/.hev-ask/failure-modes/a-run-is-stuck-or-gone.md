---
id: "failure-modes#a-run-is-stuck-or-gone"
title: "Failure modes"
heading: "A run is stuck or gone"
group: "Operate"
order: 162
url: "/docs/failure-modes#a-run-is-stuck-or-gone"
anchor: "a-run-is-stuck-or-gone"
terms: ["stuck","gone","explains","states","deal","stop","ends","harness","children","records","failed","exit","code","cleaning","finished","removes","worktree","refuses","running","leaves","worktrees","uncommitted","changes","place","keeps","record","branch","unless","admission","rework","still","whose","cleaned","dispatch","name","making","progress","check","last","lines"]
hash: "87e17557bb006103ada64c9c1976983f37e0184281e5b93a486258ee5cdd32d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"The run NAME is running but not making progress. Check the last 40 lines of `hev dispatch logs NAME` and show them to me, then end it with `hev dispatch stop NAME`. Verify with `hev dispatch ls --all` that its state is failed (exit code 143). Do not clean it.\nDocs: https://hevkit.com/docs/failure-modes#a-run-is-stuck-or-gone (all pages: https://hevkit.com/llms.txt)","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"running","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"done","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"failed","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"timeout","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"died","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"needs-review","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"stop","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"clean","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"stop running order before clean","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"rework","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":") and a run whose worktree was removed (","chunkId":"failure-modes#a-run-is-stuck-or-gone"}]
sources: [{"chunkId":"failure-modes#a-run-is-stuck-or-gone","url":"/docs/failure-modes#a-run-is-stuck-or-gone","anchor":"a-run-is-stuck-or-gone"}]
---

Explains the run states and how to deal with a run that is stuck: stop it, which ends the harness and its children and records it as failed with exit code 143. Cleaning a finished run removes its worktree but refuses a running run and leaves worktrees with uncommitted changes in place; it keeps the record, branch and PR unless the run failed admission. Rework refuses a still-running run and one whose worktree was cleaned.
