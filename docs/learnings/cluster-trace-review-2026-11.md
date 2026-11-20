# Cluster trace review, 2026-10-18 to 2026-11-17

What failed on Adam's cluster in the last 30 days, and which gate or guide
would have caught it. This is the input that
[RFC 0019 (Gates)](https://github.com/hev/kit/pull/268) asks for before its
first rollout step. It names the first gates to configure and the first goals
for the **improve** and **garden** orchestrators.

Every example below is a real run name (`host/run`) from this cluster. Counts
come from dispatch records, steering receipts, orchestrator notices and run
transcripts, read on 2026-11-17 by run `kit-cluster-trace-review-20261117`.

## Coverage

| Source | Hosts | Window actually present | Read |
|---|---|---|---|
| `hev dispatch ls --all --json` | oven, mini, win-oven | 2026-10-31 to 2026-11-17 (older records are gone) | oven 231 rows, mini 188, win-oven 134 |
| Run directories: `history.json` (every rework diff), `result.json`, `status.md` and its archived copies, `decisions.json`, `validation.json`, `verdict.json`, `report.md` first lines | oven, mini | same | 355 non-client runs, 529 attempts, 183 rework revisions |
| `hev steer ls --json` | oven, mini | 2026-11-17 only (steering shipped with PR #255 that day) | 37 receipts |
| Orchestrator `rounds/*/reply.md`, `notices/`, `last-notice.json`, checkpoints | oven, mini | 2026-11-08 to 2026-11-17 | every `[needs you]` line |
| Trace archive, `hev traces list DATE` per day | every capturing host (oven, mini, laptop) | 2026-10-18 to 2026-11-17 | 4,539 sessions: harness, host, tokens, title |
| Run transcripts on disk (Claude Code and Codex JSONL), mapped from each run's `session_id` | oven, mini | 2026-11-01 to 2026-11-17 | 257 runs had a transcript on disk (173 of 197 on oven, 84 of 158 on mini); 21,875 tool calls scanned |
| `hev board search` | hosted board | 30 days | learnings #393, #433, #435, #436, #460; tooling #468; floor #489 |

Not read:

- **win-oven**: client host. Its 134 runs are counted by state only (below).
  No transcript, order body or report was opened.
- **Client runs on oven and mini** (46 on oven, 20 on mini) were excluded by
  matching run metadata and order text mechanically; they appear only in the
  state counts. The match errs toward exclusion: a run whose order names a
  client in passing is excluded too.
- **The laptop** is not a dispatch host and can't be reached directly from
  oven, so it has no dispatch records here. Its 371 archived sessions are
  counted by size only.
- `hev traces stats` (PR #253) is not on this host yet, so tool-surface
  counts come from regular expressions over tool-call input. They overcount
  quoted text (see "Tool surface"), so read them as upper bounds.

Run states, by host:

| Host | Non-client runs | Client runs (count only) |
|---|---|---|
| oven | 150 done, 4 complete, 11 failed, 9 stopped, 4 blocked, 1 halted, 1 needs-review, 5 other | 41 done, 2 failed, 1 stopped, 2 other |
| mini | 143 done, 1 complete, 12 failed, 1 stopped, 3 blocked, 2 needs-review, 6 other | 16 done, 2 failed, 2 other |
| win-oven | none | 114 done, 12 failed, 3 timeout, 2 blocked, 1 stopped, 2 other |

## Top findings

1. **The independent reviewer is the validator that works, and it fails
   almost every first head.** 23 of 26 first reviews returned FAIL, "rework
   needed" or "changes needed". 56 reworks across 38 runs came from review
   findings. The
   formal `validation:` field was used by three runs in 30 days, all smoke
   tests, and validator infrastructure failed four times. Make review a
   `finish` validator gate.
2. **Environment failures lost whole runs.** Oven's tmux server died with
   every worker in it on five occasions: three on 2026-11-07, one on
   2026-11-09 from a worker's `tmux kill-server`, and one on 2026-11-12.
   Environment failures caused 16 reworks across 13 runs. Five more runs
   reported done with CI red or a check not run.
3. **Credentials and permissions caused the longest waits.** One deploy run
   took 26 attempts, 19 hours of waiting between them and the most tokens of
   any run. 20 of its 25 reworks were missing secrets, refused tokens or
   stop-and-resume diagnosis. Three more launches died within a second on
   1Password resolution.
4. **Most `[needs you]` notices were configuration gaps or a stuck run
   state, not decisions.** Of 128 notices, 56 came from one expediter that
   repeated the same "admission file missing" message every round for 56
   rounds. Only about one in six notices needed a person's judgment.
5. **Base drift and inconsistent branch rules** caused 15 reworks, and one
   of the thirteen `needs-decision` questions was whether to rebase or merge
   forward.

## 1. Rework causes

There were 183 rework revisions on 84 of the 355 non-client runs. Each was
classified from its diff in `history.json`.

| Cause | Reworks | Runs | Evidence |
|---|---|---|---|
| Independent review or check found defects | 56 | 38 | `oven/kit-traces-stats-20261116` (5), `oven/kit-hosted-peers-20261114` (4), `oven/kit-peers-parity-20261108` (3), `oven/kit-windows-supervise-20261105` (3) |
| Owner changed scope or requirements after seeing the work | 29 | 21 | `oven/kit-docs-07-20261111`, `oven/kit-release-train-20261111`, `oven/layer-kit-accounts-move-20261112`, `mini/mind-book-google-20261117` |
| Environment killed or broke the run | 16 | 13 | `oven/kit-member-notices-pull-20261107` (3), `oven/kit-skills-setup-20261107` (2), `oven/kit-peers-parity-20261108`, `oven/kit-asks-filters-20261112` |
| Base moved: rebase, merge forward, restack, pin bump | 15 | 15 | `oven/kit-secrets-neutral-refs-20261112`, `oven/kit-cloud-hosts-20261109`, `mini/kit-site-07-unify-20261116`, `oven/kitcloud-combine-20261114` |
| Re-review of a new head (reviewer runs) | 15 | 7 | `oven/kit-traces-stats-review-20261116` (4), `oven/kit-hooks-steering-review-20261116` (3), `mini/kit-07-ops-fixes-review-20261117` (3) |
| Credential, permission or account blocker cleared by a person | 13 | 3 | `oven/kitcloud-deploy-test-20261114` (10), `mini/media-hevc-passthrough-20261103` (2), `mini/dispatch-boat-20261031` |
| Orchestrator stop-and-resume diagnosis steps | 11 | 2 | `oven/kitcloud-deploy-test-20261114` (10), `mini/fieldkit-uat-20261115` |
| `needs-decision` answered | 10 | 8 | see section 3 |
| Order metadata only (reply line, harness) | 6 | 6 | `oven/kit-secrets-windows-token-20261113`, `oven/kit-fieldkit-api-20261114` |
| Reported done with CI red, or a CI check not run | 5 | 5 | `mini/kit-board-facets-20261117` ("you reported done at 406b398 while CI was not green"), `oven/kit-skills-setup-20261107` (`scripts/test-skill-sources.py`), `oven/kit-peers-parity-20261108` (`scripts/test-orchestrators.py`) |
| Run ended without pushing | 2 | 2 | `oven/kit-skills-setup-20261107` ("waiting for the race-suite notification"), `oven/d2048-models-probe-20261111` (timed out with untracked work) |
| Wrong external assumption | 2 | 1 | `oven/kit-opencode-capture-20261107` targeted OpenCode 1.19.5 from npm; 2.2.6 was current |
| Unhandled inbox note | 1 | 1 | `oven/kit-cloud-credits-metering-20261114` |
| Rework end-to-end test | 2 | 1 | `oven/scratch-hunk-e2e-20261117` |

### 1a. Review findings (56 reworks, 38 runs)

Review is where defects are found. The reviews caught:

- **Secrets and private data leaving the system:**
  - an API key echoed in error text (`oven/kit-cli-api-key-20261115`);
  - a PEM key archived unredacted mid-stream
    (`oven/kit-opencode-capture-20261107`);
  - local paths shown on a hosted page, which needed four rounds
    (`oven/kit-hosted-peers-20261114`).
- **Destructive local behavior:** `hev skills` setup deleted Claude's skills
  when `~/.codex/skills` was a symlink (`oven/kit-skills-setup-20261107`).
- **Hangs and resource bounds:**
  - a clone that never returned on macOS (`oven/kit-nodes-repo-add-20261107`);
  - unpruned usage entries (`oven/kit-secrets-account-budget-20261113`);
  - unbounded rate buckets (`oven/kitcloud-earned-codes-20261114`).
- **Trust boundaries:** unchecked `peers send` arguments taken from host
  JSON (`oven/kit-member-notices-pull-20261107`).
- **Measurement correctness, over six review rounds:** quoted text counted as
  executed commands (`oven/kit-traces-stats-20261116`).

Long loops were common. `oven/kit-traces-stats-20261116` has had six reviews
so far (FAIL at `3e0963c` on the latest). `oven/kit-hosted-peers-20261114`
and `oven/kit-peers-parity-20261108` each needed five before passing. In the
hosted-peers case the reviewer wrote that the filter "has now failed three
reviews, each time on forms the previous list did not name". The bar was
moving because the acceptance criterion was a list of examples rather than a
rule.

**Gate.** Review becomes a `finish` validator gate, so it is no longer an
orchestrator's private loop:

```toml
[[gates]]
name = "review"
on = "finish"
decider = "validator"
kind = "cli"                    # source review plus probes
optional = true                 # requested with validation: review
host = "oven"
harness = "codex"               # differs from the implementer pair
rubric = "~/.config/hev/rubrics/review.md"
on_fail = "rework"
max_rework = 3                  # after 3, escalate to the requester
```

**Guide.** Add a line to the order template: "State acceptance as a rule with
a test from both sides, not a list of examples." Three of the long loops were
example-list criteria: hosted-peers path filter, traces-stats command
detection, windows-supervise path semantics.

### 1b. Owner scope changes (29 reworks, 21 runs)

These are healthy. Adam saw the work and redirected it, as in
`oven/kit-docs-07-20261111` ("Adam read the seam page and the title is
wrong") and `oven/layer-kit-accounts-move-20261112` (the destination changed
twice). No gate would have prevented them. Two patterns would have made them
cheaper:

- **Docs or preview first.** `oven/kit-release-train-20261111`'s rework made
  "documented first in a preview" the train rule. The same pattern applies to
  site copy (`oven/kit-configurator-20261114`, `mini/mind-book-google-20261117`)
  and logo rounds (`mini/kit-logo-proofs-20261112`).
- **Copy decisions as a `person` gate before build.** Two reworks of
  `mini/mind-book-google-20261117` changed only wording.

### 1c. Environment kills (16 reworks, 13 runs)

| Event | Runs hit (non-client) | Cause |
|---|---|---|
| 2026-11-07, three times | `oven/kit-member-notices-pull-20261107`, `oven/kit-skills-setup-20261107`, `oven/kit-nodes-repo-add-20261107`, `oven/kit-opencode-capture-20261107` | A kit test escaped tmux socket isolation and hung up every worker |
| 2026-11-09 14:48Z | `oven/kit-short-invite-codes-20261109`, `oven/kit-peers-parity-20261108` | A worker ran `tmux kill-server` inside a pane, which hit Oven's real server because `$TMUX` was set |
| 2026-11-12 13:10Z | `oven/kit-asks-filters-20261112`, `oven/d2048-coach-resume-20261112` and others (board #460) | Cause not found; notify failed, so no FOH session was told |

Other environment failures:

- `mini/media-gemma-metadata-20261102`: timeout at 6 h (exit 137).
- `oven/fieldkit-review-phase0-20261114`: provider content flag.
- `oven/media-edit-via-dispatch-20261114`: `mise` didn't trust `wt/mise.toml`.
- `oven/layer-billing-metrics-20261114`: CI DNS failure.
- `mini/shell-trace-review-daily-20261116`: plain retry.

The fix was a rework note pasted into four runs at once: "ALWAYS run go test as
`env -u TMUX -u TMUX_PANE TMUX_TMPDIR=$(mktemp -d) go test`". kit has no
`AGENTS.md`, so that rule exists only in old orders and in board #393.

**Gate.** A `tool` gate that denies tmux server-wide commands on the default
socket:

```toml
[[gates]]
name = "no-default-tmux-kill"
on = "tool"
match = "process.kill"          # proposed class; see below
decider = "command"
deny = ["tmux kill-server", "tmux kill-session -a", "pkill -f"]
```

The `pkill -f` entry comes from the same log. `oven/kit-hooks-steering-20261116`
ran `pkill -f '^sleep 30$'` twice and killed another agent's polling loop
(reported by `orch-kit-hooks-steering` round 008). Every later rework of that
run had to say "Do not kill processes by pattern". RFC 0019 lists no class
for process signals or tmux. This review suggests adding one (`process.kill`)
next to `git.push`.

**Guide.** Create kit's `AGENTS.md` with the test-isolation line and the
`HOME`/`TMUX_TMPDIR` rule from board #374 and #393.

### 1d. Base drift and branch rules (15 reworks, 1 decision)

Every v0.7 train run that sat open while siblings merged needed a merge
forward or a rebase. The instructions contradicted each other:

- `oven/kit-cluster-status-facts-20261117` and `oven/kit-cloud-hosts-20261109`
  were told to rebase and push again.
- `oven/kit-secrets-neutral-refs-20261112`, `oven/kit-docs-sitewide-v07-20261112`
  and `oven/kit-secrets-quota-guard-20261113` were told "merge commit, no
  rebase, no force-push".
- `oven/kit-secrets-quota-guard-20261113` stopped to ask which
  (`needs-decision base`).

**Gate.** A `merge` gate with a command decider that checks the PR merges
cleanly into its base at the head being merged. That is the expediter's
current check, moved into a gate.

**Guide.** One sentence in kit's `AGENTS.md`: "Train branches merge base
forward with an ordinary merge commit. Never rebase a published branch unless
the order says so."

### 1e. Credentials, permissions and accounts (13 reworks, 11 stop-and-resume steps, 3 failed launches)

`oven/kitcloud-deploy-test-20261114` is the outlier: 26 attempts, 3 h 25 min of
work, 19 h 23 min of waiting between attempts (longest gap 14 h) and 102M input
tokens, the most of any run in the window. Its blocks, from `status.md`:

- a 1Password reference that didn't exist;
- an API refusing a client's User-Agent;
- 1Password item creation failing several times;
- a mint token refused;
- a telemetry token without permission;
- ten stop-and-resume steps diagnosing one hosted 503.

The orchestrator's own note caused one block. It left out `--category`, which
`op item create` needs; rework 22 says "The failed form was mine".

`oven/reranker-notebooks-companion-20261115`, `-b` and `-c` were launched three
times in four minutes. Each ended within a second with "1Password secret
resolution failed (service account required; desktop unlock disabled)". Their
notify failed too, so the sender learned of it only by looking.

On the mini:

- `mini/media-hevc-passthrough-20261103` and three
  `mini/resolve-activate-mini-20261102{b,c,d}` runs stopped on macOS
  permission prompts.
- `mini/fieldkit-clerk-dev-20261114` stopped because the Clerk dashboard was
  not signed in.
- `mini/dispatch-boat-20261031` stopped on a provider plan limit.

**Gate.** An `admit` gate with a command decider that resolves every secret
reference the order and its launch environment need before the worktree
exists:

```toml
[[gates]]
name = "secrets-resolve"
on = "admit"
decider = "command"
run = "hev secrets check --order $HEV_ORDER"   # labels only, never values
on_fail = "stop"
```

Item creation by a run becomes a `person` gate on a new `secret.write` tool
class, addressed to `owner`.

**Guide.** A shelf skill for the mini GUI preflight. It would check the TCC
permissions (Automation, Local Network, Accessibility), browser sign-ins and
mailbox access that an order needs before taking the desktop. Five of the
blocked mini runs above stopped on exactly these.

### 1f. Done with CI red, finished without pushing, inbox missed (8 reworks)

- `mini/kit-board-facets-20261117` reported done at `406b398` with
  `windows-inventory` failing and `go` still pending.
- `oven/kit-skills-setup-20261107`, `oven/kit-peers-parity-20261108` and
  `oven/layer-billing-metrics-20261114` went red in CI on a script the worker
  hadn't run.
- `oven/kit-docs-upgrade-api-20261114` went red on `windows-inventory` with a
  site-only commit.
- `oven/kit-skills-setup-20261107` ended its turn "waiting for the race-suite
  notification" with a local commit and no PR.
- `oven/d2048-models-probe-20261111` timed out with `docs/`, `probe/` and
  `ledger/` untracked and two inbox notes unread.
- `oven/kit-cloud-credits-metering-20261114` missed a 13:47Z inbox note.

**Gate.** A `finish` command gate, the cheapest gate in this report:

```toml
[[gates]]
name = "pushed-and-green"
on = "finish"
decider = "command"
run = "hev gate check pushed"   # worktree clean, HEAD == origin branch, PR checks green on HEAD, no inbox entry newer than the last status line
on_fail = "rework"
max_rework = 1
```

## 2. Validation

**Formal validation was almost unused.** `validation:` other than `none` was
set on three runs, all smoke tests:

- `oven/validate-smoke-pass-20261031`: pass;
- `oven/validate-smoke-fail-20261031`: fail, as intended;
- `oven/kit-expedite-smoke-20261116`.

The validator infrastructure failed four times:

| Run | Failure |
|---|---|
| `oven/kit-expedite-smoke-20261116` | Validator `-v1` on the mini recorded `fail` with no verdict at head `9ab292b`. `-v2` to `-v4` passed at the same head. One unchanged head got both FAIL and PASS, so this is a flaky validator. |
| `mini/kit-site-launch-r2-20261105-v1` | `invalid validator verdict: passing item requires evidence` |
| `orch-expedite-20261116-133028` round 001 | Oven: "validator must use configured harness and model"; mini: "validation requires a separate host" |
| `orch-expedite-20261117-181841` | `hev dispatch validate mind-reinvent-hero-20261117 --host mini` returned `unknown validation host` (no `[validation]` on the mini) |

**Independent review runs are the validator in practice.** There were about
47 review or check runs on oven and mini: `fieldkit-review-*`, `*-review-*`,
`kit-images-check-20261111` and the browser check `mini/kit-asks-uat-20261111`.
23 of the 26 first reviews (not counting delta reviews) returned FAIL,
"rework needed" or "changes needed". The three that passed first time were
`oven/fieldkit-review-combine-20261114`, `mini/kit-board-eval-review-20261117`
and `oven/review-kit-172-20261106`.

**What shipped broken, or reached a person, that a validator would have
caught:**

| Kind | Run | What happened |
|---|---|---|
| browser | `oven/kitcloud-foh-rework-20261115` | Adam's hand retest on the deployed test host produced findings 12 to 14 (first-trace overlay, unstyled pages) after review had passed. Adam was the browser validator. |
| browser | `oven/kit-ask-traces-20261111` | The browser check `mini/kit-asks-uat-20261111`, dispatched as an ordinary run, failed two acceptance items in the Asks view. That is a validator gate run by hand. |
| browser | `oven/kit-hosted-asks-board-20261114` | Review found a `TypeError` on a page with no `session` key. A browser run of that page would have shown it. |
| browser | `mini/mind-white-brand-20261117` | Text contrast of 4.43:1, below AA, was settled by a `needs-decision`. An accessibility check in a browser rubric would have flagged it before review. |
| api | `oven/kitcloud-deploy-test-20261114` | A hosted ingest returned 503 `archive_unavailable` after deploy. The cause, a request-shape defect, was found over ten diagnosis attempts. |
| api | `mini/media-edit-dispatch-deploy-20261114` | Orders generated by the merged code had no `## Acceptance` section. The deploy was rolled back. |
| cli | `oven/kit-skills-setup-20261107` | Deleted the skills tree on a symlinked root. Caught by FOH review, not by CI. |
| cli | `oven/kit-hooks-steering-rollout-20261116` | After merge #255, the mini's hevd didn't come up within 20 s of `hev update` and was rolled back. |
| cli | `mini/kit-07-train-rollout-20261117` | After auto-update, win-oven failed `hev secrets status` and `hev board boards` (host health only; this run was on the mini). |
| cli | `oven/kit-needs-you-slack-20261117` | A scratch orchestrator spec left in `~/.config/hev/orchestrators` broke `hev orchestrate ls` for every orchestrator reading oven (steering receipt from foh-hev-7). |
| computer | `mini/media-hevc-passthrough-20261103` | Safari playback could not be verified behind macOS automation prompts. A computer validator needs the GUI preflight from 1e. |
| mcp | none | No MCP server work in the window. |

The `release` transition needs a gate most: three of the cli rows are an
update reaching a host and failing there.

```toml
[[gates]]
name = "post-update-smoke"
on = "release"
decider = "validator"
kind = "cli"
host = "mini"
rubric = "~/.config/hev/rubrics/host-smoke.md"   # hevd up in 30 s, hev secrets status, hev board boards, hev orchestrate ls, on each updated host
```

## 3. Person attention

### `needs-decision` (13 asked)

| Run | Key | Waited | Person's call, or a missing guide? |
|---|---|---|---|
| `oven/kit-secrets-quota-guard-20261113` | base | 1 min | Missing guide: branch rule (1d) |
| `oven/kit-pricing-page-20261113` | preview | 1 h 19 min | Missing guide: who owns the docs preview ref (a peer had repointed it) |
| `oven/kit-hosted-peers-20261114` | chatid | 1 min | Person's call (wire-format compatibility) |
| `oven/kitcloud-hosted-views-20261114` | equality | 5 min | Missing guide: "the pinned fake is authoritative where the spec is silent" |
| `mini/fieldkit-uat-20261115` | EMAIL | 10 h 30 min | Missing environment: the mini mailbox keyring had been empty since 2026-11-05 |
| `mini/fieldkit-uat-20261115` | CAPTCHA | 10 min | Person's call (allowlist or not) |
| `mini/fieldkit-uat-20261115` | CLICK | 5 min | Person's action (solve the human check) |
| `oven/kitcloud-foh-rework-20261115` | asks | 23 min | Person's call (Adam took over browser testing) |
| `oven/kitcloud-foh-rework-20261115` | scope | 0 min | Missing guide: conflicting rework sections in one order |
| `oven/kitcloud-deploy-test-20261114` | verify | 0 min | Person's call (no temporary key in Adam's team) |
| `mini/mind-white-brand-20261117` | contrast | 28 min | Person's call (brand token) |
| `mini/kit-07-train-rollout-20261117` | winoven | open (asked 06:34Z) | Person's call (rollback or not) |
| `mini/kit-site-07-docs-20261117` | ci | open (asked 18:53Z) | Missing guide: what to do with a known flaky test (`TestHookBlockedIOFailsOpen`, 73 ms over a 5.1 s bound) |

Six of the thirteen were real decisions for a person. Six were missing guides
or missing environment, and one was a person's action. Answers came fast
while a FOH session was attached (median 5 minutes). The two still open were
asked overnight or while the owner session was busy.

### `blocked` (45 distinct reasons, 21 runs)

| Group | Count | Examples |
|---|---|---|
| Credential, permission, account | 20 | `oven/kitcloud-deploy-test-20261114` (13), `mini/resolve-activate-mini-20261102{b,c,d}`, `mini/dispatch-boat-20261031` (2), `mini/fieldkit-clerk-dev-20261114`, `mini/media-hevc-passthrough-20261103` |
| Deploy diagnosis needing logs or authority | 9 | `oven/kitcloud-deploy-test-20261114` |
| CI unavailable (runner, DNS) | 4 | `oven/layer-ce-compose-telemetry-20261113` (no runner picked up jobs; tooling #468), `oven/layer-billing-metrics-20261114` (2), `oven/kitcloud-combine-20261114` |
| Order stop condition or design question | 4 | `mini/intranet-drop-needs-you-20261102`, `mini/foh-idle-wake-20261031`, `mini/kit-board-cutover-20261101`, `mini/media-imovie-library-handoff-20261101` |
| Rollout or deploy check failed | 3 | `oven/kit-hooks-steering-rollout-20261116`, `mini/media-edit-dispatch-deploy-20261114` (2) |
| Known failing tests outside scope | 2 | `mini/mind-reinvent-hero-20261117`, `mini/mind-book-google-20261117` (site tests that need an archive a fresh worktree lacks) |
| Missing tool on host | 2 | `mini/acceptance-recorder-verify-20261111` (no headless Chromium), `mini/mini-netwatch-20261116` (needs sudo) |
| Repository access | 1 | `oven/reranker-multi-model-20261115` (hevbot is read-only on the repository) |

### `[needs you]` notices (128 records)

There were 115 records on oven and 13 on the mini, in orchestrator
`reply.md`, `notices/` and `last-notice.json`. One notice is often stored
twice, as a reply and as a notice.

| Cause | Records | Example | Person's call? |
|---|---|---|---|
| Orchestrator admission file missing on the mini, repeated every round | 56 | `orch-expedite-20261116-035359` rounds 002 to 056 | No: config. One notice was enough; 55 repeats cost about 18M input tokens (55 rounds at the 318k median) |
| Dispatch show unreachable or review not started | 24 | `orch-expedite-20261113-140702` (`reachable=false` for `kit-hevd-cluster-20261113`), `orch-expedite-20261114-145029` | No: cluster RPC |
| Validation host or validator config missing | 17 | `orch-expedite-20261116-133028`, `orch-expedite-20261116-143030`, `orch-expedite-20261116-143531`, `orch-expedite-20261117-181841`, `kit-expedite-followthrough-20261116` | No: config |
| Run stuck `running` after the harness finished | 6 | `orch-kit-hooks-steering` round 008, `orch-kit-traces-stats` round 005 | No: supervisor bug |
| Orchestrator budget used up (`runs_per_day` 6) | 3 | `orch-kit-traces-stats` round 015 | Yes, but a budget gate could ask it directly |
| CI rerun needs authority (flaky test) | 3 | `orch-kit-board-memory` (`windows-inventory`), `orch-kit-07-train2` (`TestHookBlockedIOFailsOpen`) | Guide: flaky-test policy |
| Real decisions | 19 (about 6 questions) | `orch-kit-07-train` (win-oven rollback), `orch-kit-hooks-steering` round 028 (two rollout decisions), `orch-expedite-20261116-143531` (scratch PR disposition), `orch-expedite-20261115-143047` (fork PR needs an upstream merge) | Yes |

About one record in six was a person's call. Most of the rest go away once
each notice is an open `person` gate with a stable subject that is raised
once, rather than a line rewritten every round. That is RFC 0019's attention
model.

## 4. Steering

Steering shipped on 2026-11-17 (PR #255), so there is one day of receipts:
37 in total. 4 were to client runs and are not described here.

| Level | Count (non-client) | What prompted it |
|---|---|---|
| note: peer coordination | 17 | Runs splitting files and agreeing seams: `oven/kit-07-files-archive-20261117` and `oven/kit-07-files-board-20261117`; four `mini/mind-*` runs sharing one site |
| note: from a person or orchestrator | 13 | Scope removed because another PR is rewriting the same pages (`oven/kit-07-files-board-20261117`, from foh-hev-4). A merge hold (`orch:kit-07-train2`). A cleanup request after a scratch spec broke the host (`oven/kit-needs-you-slack-20261117`). Two scope additions (`oven/kit-dispatch-slots-reply-20261117`, `mini/mind-newsletter-sheet-20261117`). |
| urgent | 2 | A guest's name and spelling from Adam (`mini/mind-talk-full-send-20261117`); the canary (`oven/steer-canary-20261117`) |
| halt | 1 | Canary only (`oven/steer-canary-20261117`) |
| redirect | 0 | |

Before steering, halts were `stop` files:

- 8 expediters and 1 review orchestrator (`o-kit-hunk-review`) on oven, and
  1 expediter on the mini, were stopped;
- `oven/kit-board-eval-20261115`, `oven/kit-pricing-usage-20261114` and
  `oven/kit-site-launch-20261105` were stopped mid-run.

No halt in the window was prompted by unsafe behavior. The unsafe actions
found here (the `pkill`, the tmux kill, the scratch spec) were found
afterwards, from reports. That points to tool gates rather than faster halts.

## 5. Tool surface

From 21,875 tool calls in 257 run transcripts on oven and mini. The patterns
match tool-call input, so quoted text counts too.

| Class | Calls | Runs | Notes |
|---|---|---|---|
| Skill loads | 983 | 172 | Mostly `peers`, `hev-dispatch` and `hev-handoff` (read at start), then `browser-use` and `acceptance-evidence` |
| Skill loads outside the hev set | 21 skill names | 23 | `build-digest` (9 runs) and `hevlayer-search-app` (2), which are repository skills. `oven/kit-opencode-capture-20261107` loaded `canary-notes` and `e2e-notes`. `mini/kit-skills-shelf-20261116` and its review loaded personal operator skills (`personal`, `poteto-mode`, `brainstorming` and others) while testing shelf isolation. `mini/shell-op-callers-20261113` loaded `granola`, `freshtime` and `nano-banana`. |
| WebFetch or WebSearch | 18 | 5 | `oven/kit-docs-07-20261111` fetched three third-party GitHub pages; `oven/kit-release-train-20261111` and `oven/kit-notify-slack-20261115` fetched vendor docs; `oven/d2048-prep-20261111` and `oven/d2048-engine-20261111` fetched model docs |
| `curl` or `wget` in shell | 478 | 67 | Mostly localhost and the run's own test host. External: openrouter.ai, huggingface.co, raw.githubusercontent.com, api.linear.app, archive.ubuntu.com |
| Package install | 282 | 85 | Mostly `npm ci` and `pnpm install --frozen-lockfile`. Unpinned or global: `npm install -g pnpm@9`, `npm i playwright@latest`, `npm install -g @openai/codex`, `sudo apt-get install` (two runs), `go get github.com/hev/kit@<sha>` |
| `git push` | 451 | 150 | 16 runs used `--force-with-lease`, mostly because a rework asked for a rebase. 1 plain force push: `oven/kit-fieldkit-client-20261114` ran `git push -f` after rebasing PR #158 onto v0.7. The 5 `--force` matches in `mini/kit-site-07-docs-20261117` are hook examples in docs text. |
| Writes to hev or harness config | 35 | 16 | Most go to a temporary `HOME` in tests or demos. Real host files: `oven/kit-hooks-steering-20261116` backed up, edited and restored the live `~/.codex/config.toml` twice. `oven/kit-needs-you-slack-20261117` left a spec in the live `~/.config/hev/orchestrators/`. |

Two lessons for the tool-gate design:

- **Classify executed commands, not text.** Every `--force` match but one and
  most "config write" matches were heredocs, docs or temp-`HOME` fixtures.
  Review of PR #253 found the same problem. A `config.write` matcher must
  resolve the path after `HOME` is overridden, or it will block every hermetic
  test.
- **Skills outside the shelf were rare and explainable.** A deny-by-default
  `skill.load` gate would have refused 23 runs. The largest group was
  repository skills (`build-digest`, `hevlayer-search-app`), which the
  repository's shelf should list.

## 6. Cost

Input tokens include cache reads, so compare them with each other, not with
a bill.

| Session class (all hosts, 30 days) | Sessions | Input | Output |
|---|---|---|---|
| Dispatch workers (includes client runs, in aggregate only) | 829 | 4.17B | 22.8M |
| Laptop interactive (FOH) | 313 | 4.63B | 14.7M |
| Mini social and drafting loops | 2,109 | 3.58B | 14.0M |
| BOH expediter shifts | 183 | 1.23B | 11.8M |
| Orchestrator rounds | 464 | 0.23B | 3.2M |
| Other | 641 | 5.04B | 19.7M |

The most expensive non-client runs by input tokens across attempts:

| Run | Input | Attempts | Why |
|---|---|---|---|
| `oven/kitcloud-deploy-test-20261114` | 102M | 26 | Credential and diagnosis stop-and-resume (1e) |
| `oven/kit-hooks-steering-20261116` | 74M | 4 | Three review reworks and a CI flake (`TestSharedCooldownAndMeter`) |
| `oven/kit-07-files-archive-20261117` | 73M | 1 | Large single build |
| `oven/kit-file-traces-retire-postgres-20261110` | 70M | 1 | Large single build |
| `oven/kit-traces-stats-20261116` | 64M | 6 | Five review reworks (1a) |
| `mini/media-gemma-metadata-20261102` | 64M | 2 | Timed out at 6 h, then continued |
| `oven/kit-peers-parity-20261108` | 55M | 6 | Three review reworks, a CI script, the 2026-11-09 tmux kill |
| `oven/kit-ask-traces-20261111` | 53M | 2 | Browser check failure (section 2) |
| `oven/kit-opencode-capture-20261107` | 50M | 5 | tmux kills and a wrong upstream version (1c) |

Abandoned or failed runs that spent tokens:

| Run | Input | Outcome |
|---|---|---|
| `oven/layer-ce-compose-telemetry-20261113` | 17.4M | Blocked: CI runner outage |
| `mini/fieldkit-uat-20261115` | 7.9M | Failed after three decisions |
| `oven/kitcloud-combine-20261114` | 6.6M | Blocked: local Docker DNS |
| `oven/reranker-multi-model-20261115` | 5.0M | Blocked: no push access |
| `oven/d2048-coach-resume-20261112` | 4.5M | Killed in the 2026-11-12 tmux death |
| `oven/kit-pricing-usage-20261114` | 3.9M | Stopped |

Thirteen runs failed before any work started. An `admit` gate would have
refused each of them cleanly:

| Runs | Cause |
|---|---|
| `oven/chart-op-vault-layer-factory-20261113`, `oven/kit-cloud-dayone-20261113` | Repository path missing |
| `oven/reranker-notebooks-companion-20261115{,b,c}` | 1Password resolution failed at launch |
| `mini/kit-cloud-hosts-20261109` | Repository not admitted |
| `mini/remove-foh-shell-20261104` | Repository not found |
| `oven/d2048-join4-20261113`, `mini/d2048-site-v2-20261112`, `mini/d2048-site-v2b-20261112` | Clone killed |
| `mini/mini-drive-check-20261031`, `mini/media-edit-via-dispatch-20261114` | `Unable to read current working directory` |
| `oven/media-edit-via-dispatch-20261114` | `mise` trust |

## Gates to configure first

Ranked by frequency times cost, highest first. Each maps to a finding above.

1. **`finish` / validator `review`** (1a, section 2). 56 reworks on 38 runs,
   and 23 of 26 first reviews failing. Moving it into a gate gives each
   verdict a head binding and a rework limit. It also removes the 15
   hand-written "Re-review: head is now ..." reworks.
2. **`admit` / command `secrets-resolve` and repository readiness** (1e,
   section 6). This covers 13 credential reworks, 11 stop-and-resume steps
   and 13 runs that failed before starting. It would have stopped
   `oven/kitcloud-deploy-test-20261114` at admission with one list of
   missing references, instead of 26 attempts.
3. **`finish` / command `pushed-and-green`** (1f). 8 reworks on 7 runs. It is
   the cheapest gate to build and catches runs that report done with nothing
   pushed, CI red or an unread inbox.
4. **`tool` / deny `tmux kill-server`, `pkill -f` and live hev config
   writes** (1c, section 5). Five mass-kill events and 16 environment
   reworks, plus two live-config edits. This needs RFC 0019 to add a
   `process.kill` class and a `config.write` matcher that follows `HOME`.
5. **`release` / validator `post-update-smoke` on each host** (section 2).
   Three updates failed on a host after merge: the mini's hevd after #255,
   win-oven's CLI checks after auto-update, and `hev orchestrate ls` broken by
   a dangling spec.

`needs-decision` as a `person` gate (rollout step 1) should ship with the
first gate. Of the 128 `[needs you]` records, 56 were one repeated message.
An open gate raised once removes that repetition.

## First goals for the orchestrators

### improve

Each goal is one recurring cause from this review, the evidence threshold that
triggers it, and the smallest change that would prevent it.

1. **Repeated `[needs you]` with an unchanged subject.** When three
   consecutive rounds of one orchestrator send the same reason, propose a fix
   to the config or the code. Seed: `orch-expedite-20261116-035359` (56
   rounds, missing admission file).
2. **Validator configuration gaps.** Any `validate` refusal ("requires a
   separate host", "unknown validation host", "must use configured harness
   and model") becomes a `hev host config` diff proposal for the owner. Seed:
   the four rows in section 2.
3. **Rework notes that repeat a rule.** When the same instruction appears in
   reworks of two or more runs, propose an `AGENTS.md` line in that
   repository. Seeds: the tmux isolation rule (four runs), "Do not kill
   processes by pattern" (three reworks), the base-branch rule (1d).
4. **Flaky tests named in decisions or notices.** Propose a fix or a
   quarantine PR with the test name. Seeds: `TestHookBlockedIOFailsOpen`
   (`mini/kit-site-07-docs-20261117`, `orch-kit-07-train2`) and
   `TestSharedCooldownAndMeter` (`oven/kit-hooks-steering-20261116`). Note
   that `kit-expedite-smoke-20261116-v1` vs `-v2` is a flaky validator, not
   a flaky test.
5. **Runs stuck `running` after the harness exits.** Seeds:
   `orch-kit-hooks-steering` round 008 and `orch-kit-traces-stats` round 005.
   This is a kit bug to file, not a guide.

### garden

1. **kit has no `AGENTS.md`.** Start the golden-principles file there:
   - test isolation (`TMUX`, `HOME`);
   - the branch rule;
   - "acceptance as a rule tested from both sides";
   - never act on the default tmux socket.
2. **Known failures in fresh worktrees.** The `mind` site has four static
   tests that fail without the weekly-update archive. They blocked
   `mini/mind-reinvent-hero-20261117` and are named as known failures in
   `mini/mind-book-google-20261117`, `mini/mind-newsletter-sheet-20261117` and
   `mini/mind-white-brand-20261117`. The mini race suite also has five macOS
   failures on plain v0.7 (steering receipt to
   `mini/kit-board-facets-20261117`). Garden PRs should make each failure skip
   with a reason or pass.
3. **Repository skills on the shelf.** Add `build-digest` and
   `hevlayer-search-app` to their repositories' shelf lists, so a `skill.load`
   gate can stay deny-by-default.
4. **Dangling host state.** Sweep scratch orchestrator specs and test
   artifacts under `~/.config/hev` on each host. Report them; don't delete
   them (that is `config.write`).
