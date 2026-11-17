# RFC 0019: Gates: checks that fail closed, set on the host and answered by a person, a command or a validator

Status: Proposed (2026-11-17). Builds on [RFC 0013](0013-hooks-and-steering.md)
(hooks and steering), [RFC 0011](0011-nodes.md) (nodes and bound senders),
[RFC 0016](0016-cluster.md) (hevd cluster), [RFC 0017](0017-notify.md)
(notify, in PR #258) and [RFC 0018](0018-skills-shelf.md) (skills shelf).
Decision: board memo #513 and the design conversation that followed it,
recorded below.

> Numbering: 0015 to 0017 are taken on open branches (host jobs, cluster,
> notify) and 0018 is the skills shelf, so this is 0019.

## Summary

A **gate** is a named check on one transition in an ask's life. It fails
closed: until a gate passes, the transition does not happen. Every gate has
one of three **deciders**: a `command`, a `validator` agent, or a `person`.
Gates are declared only in the **host's TOML**, and that file is owned by
hevd. Whoever can change the host's TOML through hevd can change its gates,
and nobody else can.

This replaces several mechanisms that kit built separately:

- repository admission;
- browser validation;
- the expediter's merge on a reviewed, green head;
- `needs-decision`;
- the "needs you" notice.

Each becomes either a gate or a gate's verdict. An open `person` gate *is*
the "waiting on you" item, so a per-person inbox comes from this primitive
rather than from its own notice type.

## Motivation

- **Kit has about six gates, each built and documented separately:**
  - admission (`[admission] repos`);
  - browser validation (`validation: browser`, `[validation]`);
  - merge-on-green's reviewed-head check;
  - the release train's "publishing is the gate";
  - `needs-decision` with `say --resolve`;
  - the peers cutover readiness markers.

  Each has its own record shape, its own refusal text and its own idea of
  who may override it.
- **Hooks look like gates but can't be one.** `hev hook` is fail-open by
  design (RFC 0013, "Fail open, always"), which is right for a sensor and
  wrong for a control. Kit needs both, and should say which is which.
- **Validation only covers the browser.** Work that changes a CLI, an HTTP
  API, an MCP server or a desktop app has no separate evaluator, only the
  worker's own receipt and CI. Separating the evaluator from the generator is
  now standard practice (Anthropic, "Harness design for long-running apps",
  Mar 2026; Böckeler, "Harness engineering", Apr 2026).
- **Attention is addressed to a host, not a person.** RFC 0017 posts every
  notice on a host to that host's single `[notify.slack] channel`. In Club
  Kit, several people send work to one host. A question needs an addressee
  and a record of who may answer it.
- **What enters a run is uncontrolled.** Skill loads, web search and fetch,
  and package installs are unrestricted today. The operator brings their own
  sandbox (see Non-goals), but kit can still refuse these at the tool
  boundary for a well-behaved agent and record what was asked for.
- **Host configuration is a plain file the agents can write.** Workers run
  as the host account and can edit `~/.config/hev/dispatch.toml` and
  `~/.hev/config.toml`. Once gates live there, the file is the policy, and
  editing it must go through an authenticated path.

## Decision

1. **Gates are host policy only.** They are declared in the host's TOML
   under `[[gates]]`. Repositories and orders cannot declare, remove or
   weaken gates. An order may *request* an optional gate the host defines
   (for example `validation: api`), but the host decides what that gate is.
   This follows admission: the receiving host decides, and sending an order
   grants no authority.
2. **Fail closed, with no exceptions.** A gate whose decider cannot run
   (missing binary, unreachable validator host, unreadable config) is
   `error`. An `error` verdict blocks the transition like `fail`. Hooks stay
   fail-open; a gate on the `tool` transition is enforced by a separate,
   fail-closed path (see "Tool gates").
3. **Three deciders:**
   - `command`: an exit code;
   - `validator`: a separate agent run against a rubric;
   - `person`: a Clerk user or role.

   `all` and `any` combine other gates.
4. **A verdict is a record bound to a subject.** Every verdict names the
   gate, the subject (a run attempt and its head SHA, a tool call, an order),
   the decider's identity and the evidence. When a new commit arrives, any
   verdict bound to the old head is invalidated, so only that head's
   verdicts count.
5. **hevd owns the host TOML.** It is the only writer. Changes go through
   `hev host config` over hevd's local socket (peer UID checks, as the
   cluster endpoint does today) or over a member's bound nodes key. Each
   change is checked against the caller's person and role, and recorded.
   hevd detects direct edits to the file and refuses to load them (see
   "Host configuration through hevd").
6. **An open `person` gate is the attention item.** RFC 0017's attention
   event gains `addressee` and `requested_by`. Notices for an open person
   gate are delivered to the addressee's routes, not the host's channel.

## Goals

- One primitive, one record shape and one refusal format for every kit
  check that blocks work.
- Validation of the browser, computer use (the desktop), HTTP APIs, CLIs and
  MCP servers, under the same rules as today's browser validation.
- Explicit host controls over skills, web access and package installs, with
  every refusal and approval recorded.
- A per-person "waiting on you" view across hosts.
- A host configuration that only authorized people can change, with a
  history of who changed it.

## Non-goals

- **Sandboxing.** The operator brings their own (BYOS): a container, a VM,
  a separate account or an egress proxy. Tool gates stop a cooperative agent
  and record the attempt. They do not contain a hostile process running as
  the host account. The docs must say this plainly.
- **Repo-declared or order-declared gates.** A repository can keep its own
  CI. Kit's gates belong to whoever runs the host.
- **Merge authority.** Orchestrators still have none. A `merge` gate decides
  whether merging is *allowed*; it does not perform the merge.
- **Replacing hooks.** Hooks remain the fail-open sensor layer. Tool gates
  reuse the hook transport, but they are not hooks.

## Vocabulary

- **Gate:** a named, host-declared check on one transition.
- **Transition:** the point in an ask's life a gate guards: `admit`, `tool`,
  `finish`, `merge` or `release`.
- **Decider:** what produces the verdict: a `command`, a `validator` or a
  `person`.
- **Verdict:** `pass`, `fail`, `waiting`, `error` or `waived`, bound to a
  subject, with evidence.
- **Subject:** what the verdict is about: an order before admission, one
  tool call, or a run attempt at a head SHA.
- **Person:** a Clerk user ID. Offline, without login, the person is the
  owner's `local:ACCOUNT` or a teammate's nodes name, as today.
- **Role:** a named set of people on the host: `owner` (always present),
  plus any roles the owner defines, such as `maintainer`.

## Transitions

| Transition | When it is checked | Blocks | Examples |
|---|---|---|---|
| `admit` | On the target, after admission's repo match and before the worktree exists | Starting the run | Repo readiness score, order requests only shelf skills, the sender's role may use this class |
| `tool` | Inside the run, at each matching tool call | That tool call | Skill load, web search or fetch, package install, writes to the host config |
| `finish` | After a clean exit and before the outcome becomes `done` | `done`: the run is reworked or ends `blocked` | Repo checks pass, validator PASS |
| `merge` | When anything (expediter, orchestrator, person) asks whether a PR may merge | Merge permission | Reviewed head, green CI, maintainer approval |
| `release` | Before a release draft is published | Publishing | The release train's checklist |

`finish` gates run in declaration order, and each `command` gate runs before
any `validator` gate. Cheap checks run first, so an expensive validator only
runs on a head that has already passed them.

## Proposed surface

### Declaring gates

```toml
# ~/.config/hev/dispatch.toml (written by hevd; see below)

[roles]
maintainer = ["adam", "user_2x9...", "alex"]   # nodes names or Clerk user IDs

[[gates]]
name = "checks"
on = "finish"
decider = "command"
run = "make check"            # in the worktree; exit 0 passes
timeout = "15m"
on_fail = "rework"            # rework | escalate | stop
max_rework = 2

[[gates]]
name = "api"
on = "finish"
decider = "validator"
kind = "api"                  # browser | computer | api | cli | mcp
optional = true               # runs only when an order asks: validation: api
host = "oven"
harness = "codex"
model = "validator-model"
rubric = "~/.config/hev/rubrics/api.md"
on_fail = "rework"

[[gates]]
name = "new-dependency"
on = "tool"
match = "package.install"     # see Tool gates
decider = "person"
addressee = "requester"       # requester | owner | role:maintainer | a person
expires = "48h"
on_expire = "fail"

[[gates]]
name = "merge-main"
on = "merge"
decider = "all"
of = ["checks", "reviewed-head", "maintainer-ok"]
```

- `optional = true` gates run only when an order requests them by name or by
  validator kind. A gate without `optional` always runs.
- An order that requests a gate the host doesn't define is refused at
  admission, as unknown shelf skills are today.

### Deciders

**`command`.** The command runs in the worktree, or on the target for
`admit`, with the run's environment allowlist. Exit 0 passes and any other
exit fails. On failure the last 4 KiB of output becomes the evidence and is
sent with a rework.

**`validator`.** This is today's browser validation, generalized. The
validator is a separate dispatch run on the configured host with a harness
and model that differ from the implementer's as a pair. It is given the
rubric, the diff, the order's acceptance criteria and a kind-specific
toolkit:

| Kind | What the validator can do | Where |
|---|---|---|
| `browser` | Drive a browser against the preview or the bootable app | Any host with a browser; mini by default |
| `computer` | Screenshots, clicks, keys and accessibility APIs against a desktop app | A host with a GUI session (the mini). Refused on hosts without one. |
| `api` | HTTP requests against the run's server, started from a declared command | The validator host |
| `cli` | Run the built binary on scripted and exploratory inputs, and compare output | The validator host, in a fresh checkout of the head |
| `mcp` | Connect an MCP client to the run's server: list tools, call each changed tool, check schemas and errors | The validator host |

The validator ends with `PASS` or `FAIL` and a findings list. Evidence
(screenshots, request and response transcripts, command logs) is stored with
the verdict. `hev dispatch rework NAME --from-validation` already sends
findings back. It becomes `--from-gate GATE`.

**`person`.** Opens a `waiting` verdict addressed to a person or role. The
addressee answers with:

```sh
hev gate pass RUN GATE [--note TEXT]
hev gate fail RUN GATE --note TEXT
```

They can also answer from the Club Kit inbox or the Slack thread. A `quorum`
(default 1) sets how many distinct people of the role must answer. When a
gate passes `expires` without an answer, `on_expire` (`fail` or `escalate`)
applies. `needs-decision` becomes a person gate the worker opens: its key
and question are the subject, and `say --resolve` is `hev gate pass` with an
answer.

### Who may answer

A verdict from a person is accepted when the person is one of the gate's
addressees. The addressees are:

- `requester`: the run's `requested_by`, inherited by reworks, validators
  and child runs;
- `owner`: the host owner;
- `role:NAME`: any member of the role;
- a named person.

Teammates who requested a run can therefore answer that run's questions.
Today `say --resolve` refuses every teammate key.

An agent acting for a person (a FOH session for Adam) records the person in
`by` and the session in `via`. A gate may set `agents = false` to require
that the answer come from a person, not through an agent session. This is
the default for `merge` and `release`, and for gates on the `config.write`
tool class.

### Waiving

`hev gate waive RUN GATE --note TEXT` is allowed only to the host owner and
to the role named in the gate's `waive_by`. A waiver is a verdict with
status `waived`, and it is never silent: it appears in `show`, the inbox and
the run's PR body.

### Tool gates

Tool gates guard tool calls inside a run. They are enforced through the RFC
0013 hook transport with one change. When a `pre-tool` event matches a tool
gate's class, `hev hook` makes a **fail-closed** gate call to hevd instead of
running fail-open handlers. If hevd cannot answer in time, the call is
blocked with `gate unavailable`. The fail-open guarantee still holds for
every event that matches no gate.

Classes are recognized from the harness's normalized tool event, using the
same tokenized command reader that urgent steering uses:

| Class | Matches | Typical decider |
|---|---|---|
| `skill.load` | A skill load that is not a shelf skill the order enabled | `command` (allowlist) or deny |
| `web.search` | The harness's search tool | Allowlist of engines, or deny |
| `web.fetch` | Fetch or browse tools, and `curl` or `wget` in shell commands | Domain allowlist |
| `package.install` | `npm`/`pnpm`/`yarn add`, `pip`/`uv add`, `go get`, `cargo add`, `brew install` and similar, plus lockfile edits that add a package | Registry and lockfile pin, or a `person` |
| `config.write` | Writes to the host TOML, hevd state, hooks or the gate rubrics | Always deny inside a run |
| `git.push` | Push, force push, and push to protected branches | `command` or deny |

```toml
[[gates]]
name = "fetch-allowlist"
on = "tool"
match = "web.fetch"
decider = "command"
allow = ["docs.github.com", "pkg.go.dev", "*.hevkit.com"]   # built-in matcher; no run needed

[[gates]]
name = "skills"
on = "tool"
match = "skill.load"
decider = "command"
allow = ["shelf:enabled", "bundled:hev-*"]
```

A short list of matchers is built in (`allow`, `deny`, `pinned`) so that
common tool gates don't fork a process on every call. For harnesses without
a pre-tool hook (pi today), tool gates cannot be enforced. A host with tool
gates refuses those harnesses at admission, saying which gate would go
unenforced.

As the Non-goals say, this stops cooperative agents and records attempts. A
shell script the agent writes and runs can still reach the network. BYOS
covers that case.

## Host configuration through hevd

Gates are only as strong as the file that declares them, so for this
purpose the host TOML moves from "a file in `~/.config`" to "state hevd
owns".

- **One writer.** hevd keeps the effective host configuration and renders
  `dispatch.toml` and `config.toml` as read-only views (mode 0444, with a
  header saying to use `hev host config`).
- **Detect direct edits.** hevd records the digest of every version it
  writes. On load, a file whose digest is not the last one hevd wrote is
  quarantined. hevd keeps running on the last good version, and a
  `config drift` attention item goes to the owner. `hev host config adopt`
  is the owner's explicit act to accept a hand edit. It is a `person` gate
  with `agents = false` on the `config` subject.
- **Authenticated changes.**
  - Locally: `hev host config set|edit|diff|log` over hevd's Unix socket,
    with peer UID checks. The caller's person comes from `hev login` on that
    machine, or from `local:ACCOUNT` offline.
  - Remotely: a new `host-config` RPC over the existing forced-command
    nodes channel, allowed to people the owner grants it to.
- **Roles over the config.** `owner` may change anything. The owner can
  grant `config:gates`, `config:admission` or `config:roles` to a role. A
  change to a gate is a diff, checked for authority and recorded in hevd's
  config log with person, via, time and digest.
- **Agents cannot change it.** The `config.write` tool class is always on
  and always denied inside a run, whatever the gates say. A config change
  made by an agent session outside a run is recorded with `via` and refused
  unless `hev host config` is called with a person who holds the right.
- **Same account, honestly.** A process running as the host account can
  still overwrite the files and hevd's state. Detecting direct edits turns
  that from a silent change into a quarantined one with an owner notice.
  Running hevd as its own system user (BYOS) prevents it. The docs give the
  recipe and do not claim more.

## The attention model

RFC 0017's `hev.attention.event/v1` gains two fields before it ships:

- `requested_by`: `{person, name}`, from the run;
- `addressee`: a list of persons or roles, from the gate.

Every open `person` verdict raises one `needs-you` event addressed to its
addressees. Delivery uses the addressee's routes (Slack user, peers inbox,
email) from the Club Kit team when the host is logged in, and falls back to
the host's `[notify.slack] channel` when it isn't.

`hev inbox` lists the open person gates addressed to you, across the hosts
you can reach:
- from each host's notice pull when offline;
- from `GET /v1/attention?for=me` on the Club Kit team Worker when logged in.

## Records

Each run gains `gates.json` beside `result.json`:

```json
{
  "gate": "api",
  "on": "finish",
  "decider": "validator",
  "subject": {"run": "improve-help", "attempt": 3, "head": "3e1c140"},
  "status": "fail",
  "by": {"run": "improve-help-validate-api-2", "harness": "codex", "model": "validator-model"},
  "via": null,
  "at": "2026-11-17T22:41:09Z",
  "evidence": ["evidence/api/transcript.jsonl", "evidence/api/findings.md"],
  "note": "POST /v1/notes returns 500 for an empty body"
}
```

- `hev dispatch show` lists every gate's latest verdict for the current head.
- The asks view draws each gate as a span.
- `hev traces ls --gate api --status fail` finds runs by verdict.

That last query is the input the improvement and gardening orchestrators
read.

## What this replaces

| Today | Under gates |
|---|---|
| `[admission] repos` | Unchanged: admission's repo match runs first and is not a gate. Readiness and class checks become `admit` gates. |
| `validation: browser`, `[validation]` | An optional `finish` validator gate of kind `browser`. `[validation]` is read as that gate for one release, then removed. |
| `hev dispatch validate`, `--from-validation` | `hev gate run RUN GATE`, `--from-gate GATE`. The old forms stay as aliases. |
| `needs-decision KEY: question`, `say --resolve` | A worker-opened `person` gate. `--resolve` maps to `hev gate pass` with an answer. |
| Expediter reviewed-head and green CI | A `merge` gate the expediter asks for permission. |
| `[needs you]` notices | The attention event for an open `person` gate. |
| Release train's publish gate | A `release` gate with `decider = "person"`, `agents = false`, `addressee = "owner"`. |

## Rollout

1. **Records and deciders.** Add `gates.json`, the `command` and `person`
   deciders on `finish`, and `hev gate`. `needs-decision` maps onto person
   gates. Add `requested_by` and `addressee` to the attention event (land
   them in #258 if it has not merged).
2. **Validators.** Generalize browser validation into the validator decider
   and add the `api`, `cli` and `mcp` kinds, then `computer` on the mini.
3. **hevd-owned config.** Rendered views, digest quarantine,
   `hev host config`, roles and the config log. Tool gates must not ship
   before this, or the file that declares them is the agent's to edit.
4. **Tool gates.** The fail-closed hook path, the built-in matchers and the
   `skill.load`, `web.fetch`, `package.install`, `config.write` and
   `git.push` classes.
5. **People.** Clerk-backed persons and routes, `hev inbox` and the Club Kit
   attention endpoint (with a kit-cloud companion RFC).

Before step 1 and in parallel with it, the trace review on the cluster
(below) sets the first real gates.

## Dogfooding: improvement and gardening as orchestrators

These are documented orchestrator examples, not new primitives. Both read
gate verdicts and traces, and both produce PRs that pass the same gates as
any other work.

- **Improve.** A repeating orchestrator reads the last N days of failed and
  reworked verdicts, steering halts and person-gate questions. It groups
  them, and for each recurring cause proposes the smallest harness change
  that would have prevented it as a PR: an AGENTS.md line, a context recipe
  change, a shelf skill, or a new gate. It never changes host config
  itself; a gate proposal is a `hev host config` diff for a person to
  apply.
- **Garden.** A repeating orchestrator scans a repository against a short
  golden-principles file kept in the repository and opens small cleanup PRs
  through the normal gates.

The first goals for both come from a trace review of Adam's cluster (laptop,
mini, oven and win-oven). That review covers the last 30 days of rework
causes, validation FAILs, `[needs you]` reasons, steering halts and
abandoned runs. Its report goes to the board, and the examples in
`docs/orchestrators.md` are written from it.

## Open questions

1. **Fail-closed latency on `tool`.** One local socket round trip per
   matching tool call is a few milliseconds. Is a 2 s budget before
   `gate unavailable` right, and should built-in matchers be evaluated in
   `hev hook` itself from a hevd-signed policy snapshot, so hevd isn't on
   the hot path?
2. **Default gates.** Should a fresh host ship with `config.write` deny
   only, or also with a default `finish` gate that runs the repository's
   declared check command when one exists?
3. **Validator cost.** Does a validator run count against the requester's
   slot and budget, the host's, or a separate validation class?
4. **Cross-host verdicts.** A `merge` gate on the laptop asks about a run on
   oven. Should verdicts be replicated through the cluster, or always read
   from the run's host?
5. **Person identity offline.** Without a Clerk login, `local:ACCOUNT` and
   nodes names are the only persons. Is that enough for a two-person team
   on one host without Club Kit, or is login required for roles?
6. **Waiving `config.write`.** Should the owner be able to waive it for one
   run (for example, a run whose job is to change hev configuration), or
   must config changes always go through `hev host config` outside a run?
