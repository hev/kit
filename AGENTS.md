# AGENTS.md

hev kit is a meta-harness for Claude Code, Codex, Antigravity, Pi, and OpenCode
on macOS, Linux, and Windows. A workspace binds subscriptions and authentication
to a home folder and a private network with a cluster. The cluster is the set
of machines running hevd and using SWIM for membership.

Rules for every agent run in this repository. Each one is here because a
past run broke it. Keep this file short: link to docs instead of copying them.

## Map

- [`MISSION.md`](MISSION.md): what kit is for and how additions are judged.
- [`README.md`](README.md): install, build and the command surface.
- [User docs](https://hevkit.com/docs) live in `hev/kit-cloud/site/`.
- Decisions, design memos, runbooks and learnings live on `hev board`.
- Proof packages live in `hev evidence`.
- Proposals and specs live in dispatch orders (`hev dispatch`, stored in `~/.hev/orders`).
- [Release notes](releases/) and [Layer schema policy](schemas/layer/) stay in this repository.
- [`.github/workflows/ci.yml`](.github/workflows/ci.yml): the checks a PR must pass.

## Don't damage the host

Dispatch hosts run other agents' live tmux sessions and the operator's real config.

- **Isolate tests.** Run Go tests as
  `env -u TMUX -u TMUX_PANE TMUX_TMPDIR=$(mktemp -d) go test ./...`, and give
  anything that touches hev or harness config a temporary `HOME`. Never act on
  the default tmux socket: a test that escapes it hangs up every worker on the host.
- **No mass kills.** Never run `tmux kill-server`, `pkill -f`, `killall` or any
  other kill by name or pattern. Kill only PIDs your own run started.
- **Live config is off limits.** Don't edit the host's real `~/.config/hev`,
  `~/.hev`, `~/.codex` or `~/.claude`, and never leave a spec in the live
  `~/.config/hev/orchestrators`, because one bad file breaks every
  orchestrator on that host.

## Branches

- Branches for a train (`v0.x`) take the train forward with an ordinary merge
  commit (`git merge origin/v0.x`).
- Never rebase or force-push a published branch unless the order says so.

## Where things go

- Document changes to user-facing behavior in the site source in `hev/kit-cloud`.
- Record proposed work in an order and design decisions on `hev board`.

## Writing code and tests

- **State acceptance as rules.** When you write or extend acceptance or tests,
  write each item as a rule and test it from both sides, what must be
  refused and what must be kept. A list of examples lets the next form through.
- **Keep secrets and local data in.** Error text, archives, logs and hosted
  pages must not carry secret values, keys or local paths. Add a test that
  shows they are redacted.
- **Bound what waits or grows.** Every network or subprocess call needs a
  timeout, and every cache, map or usage record needs a cap or pruning.
- **Windows.** Run `GOOS=windows go vet ./...` when you touch build-tagged
  files or process, signal or tmux code.

## Done means pushed and green

- Run the `scripts/test-*.py` (and `.cjs`) checks that cover what you changed;
  CI runs them, and the list is in `ci.yml`.
- Push the head, open or update the PR, and wait for CI on that head before
  writing `done`. A local commit with no PR is not done.
- Never rerun a failed test just to get green. A test that fails and then
  passes is a flake: name it in your report.
