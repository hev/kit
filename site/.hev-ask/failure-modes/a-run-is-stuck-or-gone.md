---
id: "failure-modes#a-run-is-stuck-or-gone"
title: "Failure modes"
heading: "A run is stuck or gone"
group: "Operate"
order: 155
url: "/docs/failure-modes#a-run-is-stuck-or-gone"
anchor: "a-run-is-stuck-or-gone"
terms: ["stuck","gone","explains","states","handle","runs","stop","progressing","recorded","failed","exit","clean","finished","worktree","refuses","running","worktrees","uncommitted","changes","keeps","record","branch","rework","whose","cleaned","dispatch","name","making","progress","check","last","lines","logs","show","verify","state","code","docs","https","hevkit"]
hash: "87e17557bb006103ada64c9c1976983f37e0184281e5b93a486258ee5cdd32d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"The run NAME is running but not making progress. Check the last 40 lines of `hev dispatch logs NAME` and show them to me, then end it with `hev dispatch stop NAME`. Verify with `hev dispatch ls --all` that its state is failed (exit code 143). Do not clean it.\nDocs: https://hevkit.com/docs/failure-modes#a-run-is-stuck-or-gone (all pages: https://hevkit.com/llms.txt)","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"running","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"done","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"failed","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"timeout","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"died","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"needs-review","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"stop","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"clean","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"stop running order before clean","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":"rework","chunkId":"failure-modes#a-run-is-stuck-or-gone"},{"kind":"code","literal":") and a run whose worktree was removed (","chunkId":"failure-modes#a-run-is-stuck-or-gone"}]
sources: [{"chunkId":"failure-modes#a-run-is-stuck-or-gone","url":"/docs/failure-modes#a-run-is-stuck-or-gone","anchor":"a-run-is-stuck-or-gone"}]
---

Explains run states and how to handle stuck runs: stop a run that isn't progressing (recorded as failed with exit 143), and clean a finished run's worktree, which refuses running runs and worktrees with uncommitted changes and keeps the record, branch and PR. Rework refuses running runs and runs whose worktree was cleaned.
