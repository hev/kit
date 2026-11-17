# Review dispatched work in hunk

`hev review` opens a dispatched run's change in [hunk](https://hunk.dev), a
review-first terminal diff viewer by [modem-dev](https://github.com/modem-dev/hunk)
(MIT). The notes the worker and any review run left sit next to the lines they
explain. You leave your own notes in the same window, and `hev review send`
delivers them to the run as one anchored message. Kit doesn't include a diff
viewer of its own; it uses hunk's sidecar format and its session commands.
Tested with hunk 0.25.0.

```sh
hev review fix-ci                    # fetch, merge notes, open hunk
hev review fix-ci --with fix-ci-review-2
hev review fix-ci --print-notes > notes.json
hev review send fix-ci --dry-run     # show your notes as anchored items
hev review send fix-ci               # deliver them to the run
```

Install hunk on the machine you review from (the laptop):

```sh
curl -fsSL https://hunk.dev/install.sh | sh     # or: npm i -g hunkdiff, brew install hunk
```

`hev review` never installs hunk. Without it, `hev review` prints the review
verdict, `git diff --stat` and that install line.

## review-notes.json

Every dispatched run writes `../review-notes.json` beside `report.md`. It is
hunk's agent-context sidecar, version 1, so `hunk diff --agent-context
review-notes.json` loads it unchanged. The schema is
[`schemas/review-notes.schema.json`](../schemas/review-notes.schema.json); the
Go type and validation live in `internal/reviewnotes`.

```json
{
  "version": 1,
  "head": "8c1d2e4f…",
  "summary": "Retries transient notice routes and fixes stop cleanup.",
  "files": [
    {
      "path": "internal/expedite/stop.go",
      "summary": "Stop removes the schedule before the session.",
      "annotations": [
        {
          "summary": "Order matters: the schedule fires during teardown otherwise.",
          "newRange": [41, 58],
          "rationale": "The old order left a window where a tick restarted the session.",
          "author": "kit-expedite-20261116 claude-opus-5-5",
          "source": "kit-expedite-20261116"
        }
      ]
    }
  ]
}
```

| Field | Notes |
| --- | --- |
| `version` | Optional; `1`. |
| `head` | Kit's addition: the commit the notes describe. hunk ignores it. Review runs must set it. |
| `summary` | Short changeset summary. |
| `files[].path` | Required. The file's current path, or its old path for a rename. |
| `files[].summary` | One line per changed file. |
| `annotations[].summary` | Required. The note's headline. |
| `newRange` / `oldRange` | Optional `[start, end]`, 1-based and inclusive, `end >= start`. A note without a range attaches to the file's first hunk. |
| `rationale`, `author`, `id`, `tags`, `confidence`, `source`, `createdAt`, `markup` | Optional. `confidence` is `low`, `medium` or `high`. |

Validation follows hunk's rules (a path on every file, a summary on every
annotation, ranges of two positive integers with end >= start) plus the
documented `confidence` values. Unknown fields are allowed, as hunk allows
them.

Workers are told to:

- give a short changeset summary and one summary per changed file;
- add annotations only where rationale helps a reviewer;
- set `author` to the run name and model, and `source` to the run name.

A review run writes each finding as an annotation tagged
`["finding", "fail"]` or `["finding", "note"]` with a `confidence`, and sets
`head` to the SHA it reviewed. The `VERDICT: PASS|FAIL head=SHA` first line of
its `report.md` stays as it was.

Notes are optional. A missing or invalid file never fails a run.
`hev dispatch show NAME` ends with a line such as
`review notes: /home/me/dispatch/NAME/review-notes.json (4 files, 2 annotations at 8c1d2e4f0a1b)`
or the validation error, and `hev dispatch show NAME --json` adds
`review_notes: [{path, state, error, head, files, annotations}]` (`state` is
`missing`, `valid` or `invalid`) and the raw file under
`files["review-notes.json"]`.

## hev review RUN

`hev review RUN [--host H] [--with REVIEW_RUN] [--print-notes] [--repo DIR] [--web]`
runs on your machine:

1. **Fetch.** It finds the run in the dispatch inventory (`--host` when the
   name is on several hosts) and asks the run's host, over the same SSH RPC
   path as `hev dispatch show`, for the branch head and its merge base with
   the order's `base`. When the branch's commits are not already local, the
   host sends a git bundle of them (only the commits after the merge base when
   you already have it). The branch lands at `refs/hev/review/RUN`. The branch
   doesn't need to be pushed. The repository used is `--repo`, else the
   current checkout when it already has the merge base, else a cache clone
   under your user cache directory (`hev/review/repos/REPO`). The host must
   run a hev with this feature.
2. **Merge notes.** It reads the build run's `review-notes.json` and the
   notes of its review runs: runs named `RUN-review…`, and runs with
   `review` in their name whose `base` is the run's branch (the usual
   independent review order), or only the runs named with `--with`. Notes
   whose `head` (or, failing that, the review report's `head=`) differs from
   the fetched head are dropped with a warning. A review without any head is
   kept with a warning. The build's files come first, then files only a
   review mentions; review annotations follow the build's on the same file;
   each note keeps its `author` and `source`.
3. **Open hunk.** It writes the merged notes to
   `hev/review/RUN/merged.json` under your user cache directory and runs
   `hunk diff MERGE_BASE...HEAD --agent-context merged.json --agent-notes`
   in that repository. Review verdict lines are printed first. It also
   records that repository in `hev/review/RUN/repo`, so `hev review send`
   reads the window it opened.

**Browser mode.** hunk 0.25.0 has no browser mode: `hunk diff` rejects
`--web`. `hev review --web` checks `hunk diff --help` before fetching and,
when `--web` isn't listed, stops with
`hunk 0.25.0 has no browser mode (hunk diff --web)` instead of starting a
review that would fail. It passes `--web` through only to a hunk that lists
it. For a browser view today, use `--print-notes` with another viewer, or
review the pushed PR on GitHub.

`--print-notes` writes the merged JSON to stdout and stops, for other
viewers. It doesn't fetch the branch.

## hev review send RUN

Leave notes in the hunk window: `c` on a line adds a note, and `R` replies to
the selected note. In hunk 0.25 replies work on live comments and your own
notes; the worker's and reviewer's sidecar notes don't take replies, so answer
one with `c` on its line. Then:

```sh
hev review send RUN --dry-run
hev review send RUN
```

`send` reads one live hunk window, and only one that shows the run's head in
the run's repository. The repository is `--repo DIR`, else the one
`hev review RUN` last opened for the run, else the current checkout (when it
has the run's merge base) or the review cache clone. Two clones can hold the
same commit, so a window in another repository is never read, even when it is
the only one showing that head: `send` stops and names it instead. With
`--session ID`, that window must show the run's head, and must be in `--repo`
when you pass one. Then `send` reads
`hunk session comment list --type all --json`, and keeps your notes and your
replies. Each becomes an item `path:line` (or `path:start-end`, marked
`(old side)` for deleted lines) with its text; a reply quotes the note it
answers. It prints the message, then delivers it as one message:

- `hev steer RUN --level urgent` when the run is live and this build has
  `hev steer` (detected at run time);
- otherwise `hev dispatch say RUN`, which reaches a live run's inbox and
  reworks an ended run on the same branch and PR.

Each delivery is recorded in `hev/review/sent.json` under your user cache
directory, by target (`host/run`), hunk session and note ID, with a digest of
the note's anchor and text. A later `send` delivers only notes that are new or
edited since, and says how many it skipped; when nothing is new it sends
nothing. A failed delivery records nothing, so the next `send` retries those
notes. `--dry-run` neither sends nor records. `--resend` sends every note
again. A lock file beside the ledger keeps two concurrent sends from
delivering the same notes.

hunk records notes added through `hunk session comment add` as agent
comments, not yours. Pass `--include-live` to send those too, for notes
added from a shell or script. Sidecar notes are never sent back.

```text
Review notes for fix-ci at 8c1d2e4f0a1b from hunk (2):

1. internal/expedite/stop.go:44: Keep the old order behind a flag for one release.
2. internal/expedite/stop.go:52 (reply to kit-expedite-review claude-sonnet-5-5 note "Race on schedule removal"): Agree, fix this before merge.
```

## Credits

[hunk](https://hunk.dev) ([github.com/modem-dev/hunk](https://github.com/modem-dev/hunk), MIT)
provides the viewer, the agent-context sidecar format and the live session
commands. Kit adds the `head` field, the merge, and delivery to dispatch runs.
