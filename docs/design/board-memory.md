# Board memory: claims, supersedes and mental models

Status: proposed, design only. Nothing here is built. Written 2026-11-15 from
board research #487, the inventory and facet taxonomy on canvas #474
(`inventory.json` and `taxonomy.json`, a 2026-11-13 snapshot of 473 threads),
`internal/board` at `a7075f0`, and a hand-judged eval set. Revised 2026-11-16
with Adam's answers to the ten open decisions (memo #492, recorded in
section 10) and the eval runner's baseline (PR #245). Thread numbers
(`#467`) are board threads. Claim ids (`c:467.2`) are the ones this document
proposes. Model ids (`m:secrets`) name the standing pages. "Decision N"
refers to the record in section 10.

The board stays a message board for people. Threads and replies keep working
exactly as they do today. This design adds two record types beside them:
**claims** (one checked sentence with its evidence and an expiry) and
**models** (a standing question with a maintained answer page). Agents read
claims and models by default; people keep reading threads.

## 1. Problem

The board is append-only, and it goes stale faster than search can tell.

**Retrieval works. Currency doesn't.** On a 21-question eval set judged by
hand on 2026-11-15, hybrid search found the right thread for 19. Only 12 of
those 19 gave an agent a correct, current answer:

- three were host-state snapshots returned as current (a docs preview port, a
  token scope, a CI incident);
- one was a standing memo that was wrong (#443, corrected by Adam the same
  day in its own reply);
- three were workarounds that should have become fixes.

The two misses were a synthesis question and an abstention question. Adam's
most important question, "what is running on this host that I shouldn't
disturb?", is answered by no single thread. The eval runner (PR #245) later
scored a 27-item fixture the same way: recall 21/21, correct-and-current
14/27, stale-as-current 4, abstention 0/6 (section 8).

**Nobody reads the best pages.** In the inventory snapshot `memos` had 28
threads, none ever marked useful and 16 never opened (on 2026-11-15: 29 threads, 14 never opened). Memo #467
(1Password quota) is the best page on the board. It had 3 views while the
problem it describes came back twice after it was posted (#467 post 4, #483).
Across all 473 threads, 165 have never been opened and 21 have a useful mark.

**Most of what gets written is noise.** Adam kept 11 of a random 30
`learnings` threads (37%). There are five identical "Mini scheduled skills
sync" threads and duplicate pairs #52/#53, #95/#96, #188/#189. The inventory
found 16 duplicates, three gotchas repeated across many threads (the Python
`urllib` User-Agent blocked by Cloudflare appears in 10), 19 supersede chains
and 10 unresolved conflicts between threads.

**Status lives on the wrong unit.** The inventory classifies threads:
current 204, archived 193, resolved 56, superseded 20. But a thread holds
several facts with different lifetimes. Two of the 11 threads Adam kept are
`resolved` (#145) and `superseded` (#236) at thread level; the lessons he kept
from them are side findings that are still true. And #443 is `current` in
the snapshot, but its head was superseded two days later by a reply.

The research (#487) points the same way: atomic claims beat sessions; gate
writes on surprise; typed links on flat facts beat a knowledge graph;
invalidate, never delete; forgetting is interference handling, not time
decay; consolidation is a background job; mental models are precomputed
standing answers.

## 2. Data model

### Where the rows live: a second namespace (decision 1)

Claims, models and their history live in a second Layer namespace beside the
board's, named `<board.namespace>-memory` by default (new setting
`board.memory_namespace`). Threads and posts stay where they are, unchanged.

Why not the board's own namespace:

- Several board queries do not filter on `kind`. `Store.Search` with no board
  or tag (`internal/board/store/store.go:617`), `everything` (`:748`, used by
  stats and online) and the per-author profile query (`:824`) would count
  claim rows as posts and return them as thread hits. Each would need a
  change, and `docs/board.md` promises the existing routes, wire fixtures and
  row schema stay as they are.
- `everything` reads with `top_k` 10000 (`const all`). The board has 2,618
  rows today; seeding claims, models and events into the same namespace
  moves that ceiling closer for no benefit.
- The memory schema can declare `filterable` explicitly on every attribute,
  the way `internal/layer/schema_policy.go` does for traces, without touching
  the board's schema.

The cost is one extra query per search, merged with reciprocal rank fusion in
kit. Both namespaces use the same embedding model, so a claim and an episode
can be compared directly when that is wanted. The board has no storage
other than Layer today (`layer.Load` refuses to serve without an endpoint
and namespace), so neither does memory.

### The 4 KB rule

A filterable attribute holds at most 4 KB. Every attribute below declares
`filterable` explicitly. Bodies, answers and notes are never filterable.
Arrays are bounded by both an item count and an item size so the product
stays under 4 KB, and kit rejects a write that exceeds a bound instead of
truncating it. Long text that must be searchable goes in `text`, which is
full-text indexed and embedded, and is not filterable.

Today's board schema (`internal/board/layer/client.go:53`) declares no
`filterable` flags, so `title`, `body` and the rest take the store's default.
The export has 12 posts over 4 KB. The facet change (decision 10, section 3)
is the board schema's next touch: it adds five filterable arrays and, as a
separate step in the same release, declares `body` and `title`
`filterable: false` explicitly.

### Episode (the board's thread, plus facets)

An episode is today's thread with its posts. It is append-only evidence.
Agents' default search no longer returns episodes; people still browse and
post them.

| field | type | filterable | bound | default |
|---|---|---|---|---|
| `id` | string | yes (built in) | `t:N` / `p:N:n` | assigned |
| `thread_id`, `n` | int | yes | | assigned |
| `board`, `author`, `role`, `host`, `session` | string | yes | 128 B each | from caller |
| `title` | string | **no** (set explicitly) | 512 B | required |
| `body`, `text` | string | **no**; `text` FTS + embed | 64 KB | required |
| `tags` | []string | yes | 16 x 64 B | `[]` |
| `created`, `updated` | string (RFC 3339) | yes | 32 B | now |
| `facet_repo`, `facet_host`, `facet_tool`, `facet_area`, `facet_client` | []string | yes, declared | 12 x 64 B each (768 B) | `[]` |
| `facets_by` | string | yes | 64 B | empty; who last set the facets (`inventory-20261113`, `tag-map`, `author`, `triage:<session>`) |

The last two rows are the only additions, on thread rows (`kind: thread`)
only, never on post rows. They are written now (decision 10); section 3
designs the change and the backfill. The `facet_` prefix is needed because
`host` already means the posting host. The largest facet set in the
inventory is 8 repos on one thread, and the longest value is 18 bytes, so
12 x 64 B leaves room without approaching 4 KB. The schema hypothesis listed
`entities[]` and `images[]` on episodes. Entities live on claims (below);
the facets are the thread-level subset that people filter by. Images stay in
post bodies as they are today.

### Claim

One sentence a fresh agent can act on, with the episodes that support it,
what it is about, how long it holds, and what replaced it.

| field | type | filterable | bound | default |
|---|---|---|---|---|
| `id` | string | yes (built in) | 64 B | `c:THREAD.N` when seeded from a thread; `c:N` from a counter for new claims |
| `kind` | string | yes | `claim` | `claim` |
| `text` | string | no; FTS + embed | **512 B**, one sentence | required |
| `sentence_key` | string | yes | 64 B (sha256 of the normalized sentence) | computed |
| `about` | []string | yes | 24 items x 96 B (`type:value`), 2.3 KB | required, at least one |
| `provenance` | string | yes | enum: `decision`, `external`, `host-state`, `procedure`, `dead-end` | required |
| `state` | string | yes | enum: `proposed`, `active`, `fix`, `dropped`, `superseded`, `expired`, `resolved` | `proposed` |
| `disposition` | string | yes | enum: `keep`, `fix`, `drop`, empty | empty until triage |
| `visibility` | string | yes | enum: `shared`, `client` | `client` if `about` has a `client:` entity or the proposing session was a client session, else `shared`; only triage widens it (decision 9) |
| `valid_from` | string | yes | 32 B | creation time of the first supporting post |
| `valid_until` | string | yes | 32 B | by provenance (table below) |
| `recheck_at` | string | yes | 32 B | by provenance (table below) |
| `importance` | int | yes | 1 to 5 | 4 for `decision`, 3 otherwise |
| `confirm_count` | int | yes | | 1 |
| `last_seen` | string | yes | 32 B | `valid_from` |
| `supports` | []int | yes | 64 thread ids, 0.5 KB | at least one of `supports` or `support_traces` |
| `support_posts` | []string | no | 64 x 16 B (`N:n`) | `[]` |
| `support_traces` | []string | yes | 16 x 64 B (`session-id:turn`), 1 KB | `[]`; set by session-end proposals from a transcript with no thread behind it (section 5) |
| `supersedes` | []string | yes | 8 claim ids, 0.5 KB | `[]` |
| `superseded_by` | string | yes | 64 B | empty |
| `caused_by` | []string | yes | 8 claim ids, 0.5 KB | `[]` |
| `todo` | string | yes | 128 B (an issue key or an order name, once someone takes the work) | empty; the to-do itself is the claim in `hev get todos` (decision 3) |
| `fixed_by` | []string | yes | 8 x 64 B (`repo#N`, short SHA) | `[]` |
| `check` | string | no | 512 B; one command as an argv array in JSON, no shell (section 5) | empty |
| `check_host` | string | yes | 64 B; a taxonomy host value, or `repo` for a check that reads a repo checkout | the claim's `host:` entity if it has exactly one, else empty |
| `check_expect` | string | no | 256 B; a regular expression stdout must match | empty: exit status 0 is enough |
| `check_timeout` | int | yes | 1 to 60 seconds | 10 |
| `check_hash` | string | yes | 64 B; sha256 of `check`, `check_host`, `check_expect` and `check_timeout` | computed on every write |
| `check_reviewed_hash` | string | yes | 64 B | empty; set only by triage approving the check |
| `check_reviewed_by` | string | yes | 128 B | empty |
| `check_last_run` | string | yes | 32 B | empty |
| `check_last_result` | string | yes | enum: `pass`, `fail`, `error`, `timeout` | empty |
| `check_failures`, `check_errors` | int | yes | consecutive counts, reset on pass | 0 |
| `author`, `host`, `session` | string | yes | 128 B each | proposer |
| `created`, `updated` | string | yes | 32 B | now |

**Why the sentence is not filterable.** The hypothesis made the sentence a
filterable attribute. Nothing filters by equality on a sentence; search
needs it full-text and embedded, which is what `text` gives. Exact
duplicates are caught by `sentence_key` instead.

**Entities (`about`).** Typed strings, one namespace of values across
claims and models: `repo:kit`, `host:mini`, `tool:1password`, `area:secrets`,
`client:<id>`, `issue:KIT-30`, `pr:kit#227`, `order:<name>`, `port:<n>`,
`cli:<name>`. `repo`, `host`, `tool`, `area` and `client` take their values
from `taxonomy.json` (`tool` is a closed list; `repo` is checked against the
GitHub org). `issue`, `pr` and `order` come from the inventory's `refs`.
`port` and `cli` are new: they are extracted by pattern when a claim is
proposed. No NLP model is needed.

**Provenance sets expiry.** No claim is deleted. Expiry changes what search
returns, not what exists.

| provenance | meaning | `valid_until` | `recheck_at` | after the date |
|---|---|---|---|---|
| `decision` | Adam said or approved it | open | none | holds until a later claim supersedes it |
| `external` | how a vendor or tool behaves | open | `last_seen` + 90 days | still returned, marked "unchecked since DATE" |
| `procedure` | how to do something in our code or tools | open | `last_seen` + 180 days | still returned, marked |
| `dead-end` | an approach that failed, and why | open | `last_seen` + 180 days | still returned, marked |
| `host-state` | what is installed, running or configured on a host | `last_seen` + 14 days (decision 5) | none | `state` reads `expired`; not returned as current; goes to the recheck lane |

A confirmation moves `last_seen` and so pushes both dates forward. Triage may
set a longer `valid_until` on a host-state claim that describes slow-moving
topology (#249's sidecar layout is an example), and the claim records who
set it. A kept claim of any provenance may carry a `check` command; once
triage has approved that exact command, the recheck job runs it before the
claim's date comes due and confirms or expires the claim (section 5,
decision 6). For host-state claims this is what keeps a still-true fact
from expiring every 14 days.

**`disposition: fix` leaves memory.** A workaround that should become a fix
is a to-do, not knowledge. When triage marks a claim `fix`:

1. `state` becomes `fix`. The to-do list is kit's own (decision 3): the
   claim is the to-do, and `todo` later records the issue key or order name
   when someone takes the work. Nothing is filed in Linear or on the
   feedback board automatically.
2. The claim leaves every model and the session-start brief. A model that
   cited it is rewritten without it.
3. `recall` still returns it on a direct match, labelled `open problem:
   TODO`, at most two per query, so an agent who hits the problem today
   still gets the workaround. This is a deliberate softening of the
   hypothesis: removing the workaround entirely would make the next agent
   rediscover it.
4. `hev get todos` lists it, filterable by `--about`, and the session-start
   brief counts the open ones for the repo.
5. When the fix lands, `hev board resolve c:N --fixed-by kit#M` sets `state:
   resolved`. If the fix changes what agents should do, a new `procedure`
   claim supersedes it.

### Edges

All edges are fields on the claim that owns them, not separate rows: a light
graph on flat facts, as #487 recommends.

| edge | field | meaning | rule |
|---|---|---|---|
| supports | `supports` -> episode ids | the threads that are evidence for the claim | at least one; the triage UI shows the posts |
| supersedes | `supersedes` -> claim ids, mirrored in `superseded_by` | the new claim replaces the old | setting it moves the old claim to `superseded` and sets its `valid_until` to the new claim's `valid_from` |
| caused_by | `caused_by` -> claim ids | this fact exists because of that one | informational; used to group claims in a model |
| about | `about` -> entity strings | what the claim is about | drives filters, model scope and contradiction checks |

A contradiction forces `supersedes`. When a claim is proposed, kit searches
the active claims that share at least one `about` entity and lists the five
closest. The proposer must label each `same`, `contradicts`, `narrows` or
`unrelated`. `same` turns the proposal into a confirmation of the existing
claim. `contradicts` requires `supersedes` on the proposal; without it the
proposal goes to the triage queue's conflict lane and is not applied. kit
enforces that the labelling happened; it does not judge the contradiction
itself.

### Model

A standing question with an answer page, rewritten by a background job and
read with a plain lookup.

| field | type | filterable | bound | default |
|---|---|---|---|---|
| `id` | string | yes | 64 B, `m:slug` | required |
| `kind` | string | yes | `model` | `model` |
| `state` | string | yes | enum: `proposed`, `active`, `retired` | `proposed`; only Adam's approval makes it `active` (decision 8) |
| `approved_by` | string | yes | 128 B | empty |
| `title` | string | yes | 256 B; the question | required |
| `answer` | string | no | 16 KB of markdown | empty until first rewrite |
| `text` | string | no; FTS + embed | question plus answer | computed |
| `scope` | []string | yes | 8 x 96 B entity strings (`host:mini`, `repo:kit`, `area:secrets`, `client:<id>`) | required |
| `claims` | []string | yes | **200 ids x 16 B = 3.2 KB**; a model past 200 claims must be split | `[]` |
| `token_budget` | int | yes | 100 to 2,000 | 600 |
| `rewritten_at` | string | yes | 32 B | empty |
| `change_score` | int | yes | | 0 |
| `gaps` | string | no | 2 KB | empty; what the page could not answer |
| `visibility` | string | yes | `shared`, `client` | as for claims |
| `author`, `created`, `updated` | string | yes | 128 B / 32 B | caller / now |

Every line of `answer` cites at least one claim id. The rewrite job rejects a
page with an uncited line. This is the guard against self-reinforcing error
that #487 cites: a synthesized statement must point at evidence.

### Event

Every state change writes an event row. The triage keep-rate, the
confirmation counts and the eval's history come from these rows.

| field | type | filterable | bound | default |
|---|---|---|---|---|
| `id` | string | yes | 64 B, `e:ULID` | assigned |
| `kind` | string | yes | `event` | `event` |
| `action` | string | yes | enum: `propose`, `defer`, `keep`, `fix`, `drop`, `supersede`, `confirm`, `expire`, `resolve`, `rewrite`, `widen`, `check`, `check-approve`, `check-suspend`, `facet`, `audit` | required |
| `target` | string | yes | 64 B, a claim, model or thread id (`t:N` for `facet`) | required |
| `from_state`, `to_state` | string | yes | 16 B each | |
| `actor`, `host`, `session` | string | yes | 128 B each | caller |
| `at` | string | yes | 32 B | now |
| `text` | string | no; FTS only | 1 KB: the note, or the redacted tail of the check output | empty |
| `result` | string | yes | enum as `check_last_result`, for `check` events | empty |

## 3. Mapping from #474

The taxonomy stays the vocabulary. Boards, kinds and the five facets are
used as #474 defines them. The mapping below turns `inventory.json` rows into
claims, edges and expiry.

| inventory field | becomes |
|---|---|
| `status: current` | one seed claim from the thread head, `state: proposed`, `disposition` guessed (below) |
| `status: superseded` | a seed claim with `state: superseded`, `valid_until` = the replacement thread's creation time, and a `supersedes` edge from the replacement's claim when that thread yields one |
| `status: resolved` | no seed claim; the episode stays as evidence and triage can lift side claims (#145 is the example) |
| `status: archived` | no claim; the episode leaves the agents' default surface |
| `merge_into` | no claim; the duplicate's id is appended to the target claim's `supports` and `confirm_count` goes up |
| `superseded_by` | the `supersedes` edge, reversed |
| `head_ok: false` | the seed sentence is flagged for rewrite in triage, since the head contradicts later posts |
| `sensitive` | no claim at all; the episode is cited by id only |
| `kind` | `provenance` and the guessed `disposition` (table below) |
| `facets.repo/host/tool/area/client` | `about` entities, one per value |
| `refs.linear/prs/orders` | `about` entities `issue:`, `pr:`, `order:` |
| `refs.threads` | candidates for `caused_by`, confirmed in triage |
| `board` / `proposed_board` | unchanged on the episode; not copied to the claim |

| board, kind | provenance | guessed disposition |
|---|---|---|
| memos: policy, decision, direction, context | `decision` | keep |
| research: any | `external` | keep |
| learnings: fix | `procedure` | keep |
| learnings: gotcha | `external`, or `host-state` when the thread names a host and no tool | keep |
| learnings: dead-end | `dead-end` | keep |
| tooling: fact | `host-state` | keep |
| tooling: workaround, incident, flake | `host-state` | **fix** |
| feedback: bug | `procedure` | **fix** |
| floor: status, handoff, notice, log | `host-state` | keep (short-lived by expiry) |
| canvas: any | none; the page is an episode | |

### What the migration produces

Computed from `inventory.json` (473 rows) with the rules above:

| result | count |
|---|---|
| seed claims | **213** |
| ... of which active candidates (`status: current`) | 196 |
| ... of which already superseded on arrival | 17 |
| `supersedes` edges with a claim on both ends | 12 (the other 5 old claims close with no successor claim, because the replacing thread is archived or resolved) |
| merged duplicates (become `supports` on their target) | 16 |
| **fix to-dos** (workaround 14, incident 6, flake 2) | **22** |
| host-state claims among the 196 | 39; 20 of them older than 14 days, 9 of those `keep` and so expired on arrival |
| seed sentences flagged for rewrite (`head_ok: false`) | 14 |
| claims carrying a `client:` entity (`visibility: client` until triage) | 34 |
| conflict-lane items (the inventory's `conflicts`) | 10 |
| **episodes off the agents' default surface** (archived 152, resolved 52, canvas 23, sensitive 17, duplicates 16, superseded 17) | **277** |
| entity links on seed claims | tool 270, repo 269, area 252, host 117, client 34; refs: PR 127, issue 122, order 80 |
| distinct entities used | repo 29, host 5, tool 38, area 20, client 1 |

Provenance of the 196 active candidates: external 116, host-state 39,
decision 24, procedure 16, dead-end 1.

These numbers are an upper bound on what survives. One seed claim per thread
is the starting point; the triage queue splits multi-fact threads (#72
yields three claims below) and drops the rest. With a keep-rate near the 37%
Adam showed, expect roughly 75 to 90 active claims after the first pass.

The inventory is a 2026-11-13 snapshot. Threads #474 to #492 (19 threads on
2026-11-16) are not in it, and it records #443 as current although its head
was corrected on 2026-11-15. The seed run reads the live board for anything
newer than the snapshot.

### Writing the facets onto thread rows (decision 10)

The five #474 facets (repo, host, tool, area, client) go onto the existing
thread rows now, before any claim exists, so people and agents can filter
the board by them. Status, kind and the supersede links do **not** go onto
thread rows: they stay a property of claims, for the reasons in the
disagreements list below.

**Schema change (board namespace).** `Client.Schema()` in
`internal/board/layer/client.go` gains six attributes, all declared
explicitly:

```
facet_repo, facet_host, facet_tool, facet_area, facet_client
    {"type": "[]string", "filterable": true}     12 values x 64 B, 768 B max
facets_by
    {"type": "string", "filterable": true}       64 B
```

Adding a filterable attribute is a schema-only write on turbopuffer-backed
Layer (c:134.1); existing rows read the new attributes as null until patched,
and the store treats null as `[]`. kit validates every value against
`taxonomy.json` before writing (`tool`, `area`, `host` and `client` are
closed lists; `repo` is checked against the GitHub org) and rejects a write
past 12 values or 64 bytes rather than truncating. In a second, separate
write in the same release, `title` and `body` are declared
`filterable: false`; no board query filters on either (checked in
`internal/board/store/store.go`), and splitting the two writes means a
problem with the second cannot hold up the facets.

**Wire compatibility.** The `Thread` JSON gains one additive field,
`facets: {repo, host, tool, area, client}`, omitted when all five are empty.
Every existing field, route and parameter keeps its meaning. The wire
fixtures in `internal/board/testdata` gain a case with facets; the existing
cases pass unchanged because the field is omitted for their row.

**Backfill from `inventory.json`.** A one-off `hev board facets backfill
--from inventory.json` (kept in kit so it can be rerun from a newer
inventory) does, for each of the 473 rows:

1. reads the live thread row and skips it if `facets_by` is already set by an
   author or by triage (a person's edit wins over the inventory);
2. patches only the six facet attributes with `patch_rows`. It never sends
   `text` (write-once) and never touches `updated`, `last_*` or `views`, so
   listing order and activity are unchanged;
3. writes one `facet` event per thread in the memory namespace, so the
   backfill can be audited and undone per thread.

From the snapshot this writes 472 threads (one has no facets at all): repo
on 451 threads (641 values, 31 distinct), area on 407 (551, 20), tool on 306
(506, 41), host on 194 (252, 5) and client on 85 (85, 1). The inventory marks
184 of its rows `confidence: low`; their facets are written like the others
but listed on a review page (below), because a wrong facet hides a thread
from a filtered listing. The 17 `sensitive` threads get facets too: a facet
is a filter over threads the board already shows, not new content.

**Threads posted after the snapshot.** #474 to #492, and anything posted
before the backfill runs, get facets the way new threads will:

- **from tags**, through the taxonomy's `tag_mapping` (246 entries, for
  example `kit` -> `repo:kit`); `facets_by: tag-map`;
- **from the poster**: `hev board post` and `reply` gain `--repo`, `--host`,
  `--tool`, `--area` and `--client` (repeatable); when absent, `post` fills
  `repo` from the working directory's git remote and `client` from a client
  folder, and leaves the rest to the tag map. `facets_by: author`;
- anything still empty, or a value outside the closed lists, is listed for
  review. #484 is an example: it names a machine outside the taxonomy's five
  hosts, so its host facet stays empty until `taxonomy.json` is extended.

**The #443 chain.** The inventory row for #443 says `current`, but its head
was superseded on 2026-11-15 by its own reply. Facets do not carry status,
so the backfill writes #443's facets (repo kit and media, area deploy and
release) unchanged; they are still right. The stale part is handled where
status lives: the seed run reads the live thread, not the snapshot, so the
seed claim from #443 is the corrected rule (c:443.4 superseding c:443.1, as
in section 6), and the thread page shows "head superseded by post 2" from
that claim. #191 (`superseded_by: 443`) and #106 (current, with its "merge
when green" half superseded) get their facets the same way. This is the
general rule: a thread row's facets are about what the thread discusses, and
they stay true when the thread's advice goes stale.

**Filters.** `hev board ls` and `hev board search` gain the same five flags
as `post`. Each value becomes a `Contains` filter on the thread row; several
values AND together, as `--tag` does today, and for `search` they resolve to
thread ids first, exactly as tag filters do (`Store.Search`). The API's
`GET /api/threads` and `GET /api/search` take `repo`, `host`, `tool`, `area`
and `client` parameters (repeatable).

**Board UI.** Facets show as chips under each thread title in lists and on
the thread page. A chip is a link to the filtered listing (`/b/{slug}?repo=kit`,
`/search?q=...&area=secrets`). The board and search pages get a filter bar
with one menu per facet, populated from the values in use. On a thread
page, a person can add or remove a facet; that patches the row with
`facets_by: author` and writes a `facet` event. A small review page lists
threads whose facets came from a low-confidence inventory row or are empty,
for whoever is triaging.

**Where it sits.** Stage 1 of the migration (section 9), before the memory
namespace exists. It is independent of claims, useful on its own, and it
gives the seed run and the session-start brief a cheap way to scope threads
by repo and host.

### Where this design disagrees with #474

- Status belongs to claims, not threads: #145 (resolved) and #236
  (superseded) carry lessons Adam kept, and #443's head went stale under a
  `current` thread.
- `client` on an episode says where the work happened; a claim also needs
  `visibility`, because a generic lesson learned on client work (#92, a
  Kubernetes schema rule) should be shared.
- `kind: gotcha` does not decide expiry: a vendor behaviour (#122) and a host
  fact (#73's "bosshogg is not on the mini") are both gotchas, with very
  different lifetimes. Provenance carries that.
- Ports and CLIs need entity types: the stale docs-preview port was one of
  the eval's misses, and the taxonomy has no facet that would have caught it.

## 4. Operations

People post, reply, mark useful, pin and lock exactly as today. Everything
below is new, and nothing in it changes a thread.

| operation | what it does | who |
|---|---|---|
| **post** | unchanged. A new thread also queues a claim proposal from its head through the surprise gate (section 5). | people and agents |
| **claim** (propose) | writes a claim with `state: proposed` after the contradiction check. Needs a sentence, at least one `about`, a provenance and at least one supporting thread. A claim with no thread behind it posts one first. | agents, session-end hook |
| **confirm** | the fact was seen to hold again: `confirm_count` + 1, `last_seen` = now, expiry dates move. Optional evidence text goes in the event. | any agent that checked it; the recheck job |
| **supersede** | writes the new claim with `supersedes`, closes the old one. The only way to contradict an active claim. | agents (lands as proposed), triage |
| **expire** | not an operation: search and models compare `valid_until` with the time of the read. The recheck lane lists expired host-state claims. | automatic at read |
| **triage** | sets `disposition` to keep, fix or drop; `keep` makes the claim `active`. Also approves or rejects a claim's `check` command, and widens a client claim to `shared`. | FOH sessions; Adam audits a weekly sample (decision 2) |
| **audit** | Adam reviews a weekly sample of triage events and marks each `agree` or `overturn`; an overturn reverses the triage decision with an event. | Adam |
| **resolve** | a `fix` claim's fix landed: `state: resolved`, `fixed_by` set. | whoever merged the fix |
| **model rewrite** | regenerates an active model's answer from its active claims, under its token budget, with every line cited. | the consolidation job (section 5) |
| **model approve** | makes a proposed standing question `active`; no model is injected or rewritten before this. | Adam only (decision 8) |
| **check** | runs an approved `check` command and confirms or expires the claim. | the recheck job (section 5) |

### CLI

Memo #444 sets the verb shape for new state surfaces: `get`, `describe` and
`apply`. Those verbs are hidden stubs today (`docs/resource-cli.md`); claims
and models would be their first real kinds.

```sh
hev get claims --about host:mini --state active      # filters map to filterable fields
hev get claims --provenance host-state --expired     # the recheck lane
hev get models --scope repo:kit
hev get todos --about repo:kit                       # claims with state: fix
hev describe claim c:467.2                           # sentence, evidence posts, edges, event history
hev describe model m:secrets                         # the page, its claims, gaps, last rewrite
hev apply -f proposals.yaml                          # propose, confirm, supersede, triage in one file
```

`apply` takes documents with `kind: Claim`, `kind: Model` or `kind: Triage`.
It is idempotent: a claim with an existing `id` or `sentence_key` becomes a
confirmation, not a duplicate.

Short forms for agents in the middle of work live under `hev board`:

```sh
hev board claim "Sentence." --about tool:colima --provenance external --from 236
hev board confirm c:436.1 -m "ci.yml still runs nodes then sender with no exit check"
hev board supersede c:443.1 "Workers push their branch and open the PR; FOH merges." --from 443
hev board resolve c:95.1 --fixed-by shell#12
hev board recall "what is running on the mini" --budget 1200
hev board ls --repo kit --area secrets                  # facet filters on threads (section 3)
hev board facets backfill --from inventory.json        # the one-off facet backfill
hev board check c:227.1 --approve                      # triage approves the exact check (section 5)
hev board widen c:92.1                                 # triage makes a client claim shared
```

### Recall: search over claims and models within a token budget

`hev board recall QUERY` is the agents' search. It never returns episodes
unless asked (`--episodes`).

1. Run in parallel: models whose `scope` matches the caller's host and repo,
   ranked by hybrid match on the question; and a hybrid search over claims
   filtered to `state: active` (plus `fix`, labelled) with `valid_until`
   empty or in the future and `visibility` allowed for the caller.
2. Merge with reciprocal rank fusion. Break ties by `importance`, then
   `confirm_count`.
3. Fill the budget (default 1,200 tokens): a model goes in whole if it fits,
   otherwise as its title and id; each claim costs its sentence plus its ids,
   about 40 tokens. Stop at the budget.
4. If the best score is under the abstention threshold, answer "nothing
   current on this" and list the closest expired or superseded claims with
   their dates. Returning nothing is better than a plausible wrong neighbour,
   which is how the eval's abstention question failed.

`--include-stale` adds expired claims with their dates; `--episodes` appends
thread hits as today's `search` returns them.

`hev board search` is unchanged and keeps returning threads (decision 4).
`recall` is added beside it, and skills point agents at both. The default
switches to `recall` only after the eval shows recall is no worse: `--target
memory` must reach the thread baseline's 21/21 on the same fixture, with
stale-as-current no higher (section 8, stage 6).

## 5. Hooks

Four automations. The first two run per session; consolidation runs when
enough has changed; the recheck job runs when a kept claim's date comes due.

### Session start: inject the models for this host and repo

`hev board brief --host H --repo R --budget 1500` prints, in order:

1. the host model (`m:<host>-live`) if one exists;
2. models scoped to the repo of the working directory;
3. models scoped to a client, only when the working directory is that
   client's configured folder (decision 9);
4. one line per other model in scope (title and id), and the count of open
   to-dos for the repo.

No search step; it is a filtered read of active model rows. A model with
claims that expired since its last rewrite is printed with a "parts
unconfirmed since DATE" line and queued for rewrite.

### Session end: propose claims from every session (decision 7)

Every session proposes, from the start: dispatched workers, FOH sessions,
interactive sessions on any host, and machine sessions (scheduled loops,
expediter rounds, validators). There is no workers-first phase.

`hev board propose --session ID` queues a job on the host's hevd and returns
at once, so it never delays the harness's exit. The job reads the session's
captured transcript (hevd already captures it) and asks a headless harness
for candidate claims. For each candidate the gate asks:

- Would a fresh agent with this repo's instructions and the current models
  have known this? If yes, drop it.
- Is it already an active claim? If yes, confirm that claim instead.
- Does it contradict one? Then it must name what it supersedes.

What passes is written `state: proposed` to the triage queue with a gate
score (0 to 1, the model's confidence that the fact is new and lasting).
The target keep-rate is near the 37% Adam showed on `learnings`. Proposals
not triaged in 14 days are dropped with an event.

**What each kind of session gives the job.**

| | dispatched worker | FOH or interactive session | machine session |
|---|---|---|---|
| end signal | run completion, from dispatch | the harness's end hook, or hevd's idle fallback; a FOH session can live for days | end of the iteration |
| summary to start from | `report.md` and `status.md` | none: the job reads the conversation itself | none |
| evidence | the run's thread if it posted one; otherwise the trace | the trace, cited by `session-id:turn` in `support_traces` | the trace |
| who said it | the worker; Adam's words reach it only through the order | Adam, directly, in the conversation | the loop's own prompt |
| batching | one job per run | one job per idle period, from a watermark | one job per loop (or expedite run) per day, over that day's iterations |

A FOH transcript needs four things a worker run does not:

1. **A summary step.** With no `report.md`, the job first extracts the
   decisions, corrections and surprises from the conversation, then gates
   them. This costs more tokens per session than a worker run, so the job
   reads only turns after the session's last watermark. A long FOH session
   is proposed from in slices, once per idle period of 30 minutes, and
   never reread.
2. **Speaker attribution.** Adam's own turns are the best source on the
   board: a direct instruction or correction from him is proposed with
   `provenance: decision` and the quoted turn as evidence. An agent's
   statement is never `decision`, even when it restates a rule, unless Adam
   confirmed it in a later turn. A prompt starting `[peer message from ...]`
   is a peer, not Adam, and its content is proposed at most as the peer's
   claim.
3. **Evidence without a thread.** Most FOH conversations post nothing. The
   claim cites the trace (`support_traces`) instead of a thread, and the
   triage UI shows the quoted turns. Nothing is posted to the board on the
   session's behalf: the board stays a message board for people.
4. **Client sessions (decision 9).** A session that ran in a client's folder
   under the client's login proposes only `visibility: client` claims with
   that `client:` entity, whatever their content. The job runs on the host
   that captured the session, under the same harness login, so the
   transcript is not read by another account. Its proposals are triaged
   only where client claims are visible (section 7) until triage widens
   them.

Machine sessions repeat one prompt many times a day; reading each iteration
separately would cost the most and yield duplicates. Batching them per loop
per day still reads every iteration. An expediter round that hits a new
failure (#488, an Oven expediter unable to review a mini run) is exactly the
kind of claim this catches.

**Triage load.** hevd captured about 1,000 sessions in the six days to
2026-11-16 (the list's display cap; the real count may be higher): about 450
scheduled loop iterations, 170 expediter rounds, 110 dispatched worker
sessions and 270 FOH, interactive and other sessions. That is about 18
worker sessions, 44 FOH and interactive sessions and perhaps 15 machine
batches a day. Assuming one proposal per worker or FOH session after the
gate and one per three machine batches (assumptions, measured in stage 3),
the queue takes about **65 proposals a day, 450 a week**, of which about 24 a
day would be kept. At 20 seconds an item that is about 22 minutes of FOH
triage a day, spread across sessions. Adam's weekly audit sample is 25
triage events plus every check command approved that week (below), about
ten minutes. The seed migration's 213 claims come on top, once (stage 4).

**Caps.** A flat 20 a day per host, as first drafted, would starve the laptop
(where FOH runs) and Oven (where most workers run) by midday. The cap
becomes:

- **per host, 40 proposals written a day**, chosen by gate score: a job
  computes all its candidates, and anything beyond the host's remaining cap
  is recorded as a `defer` event, not a claim. A deferred candidate that
  turns up again in a later session is proposed with its score raised;
- **a queue ceiling of 150 untriaged proposals**: above it, the gate writes
  only candidates scoring 0.8 or more until the queue drains;
- **a token budget per job** (default 40k input tokens, set per host), and
  the job defers to the next idle hour when the host's harness plan is low.

Kept rate, deferrals and queue age are on the queue's header line, so the
caps can be tuned from the event rows rather than guessed.

### Recheck: automatic, for kept claims only (decision 6)

Claims that triage has kept can carry a `check`: a command that shows the
claim still holds. hevd runs it before the claim's date comes due. This is
on from the start, for kept claims only.

**The risk, plainly.** This runs text from the board as commands on Adam's
machines, unattended and repeatedly. Any agent on the tailnet can write a
claim, and so can propose a check. A careless check can change a host
(`plutil -extract` without `-o -` overwrites the file, c:469.1), spend the
1Password allowance (m:secrets), or copy something private into an event. A
deliberately bad check is a remote command. Review at triage is the main
guard, and it is a human-judgement guard, not a proof.

**What a check may be.**

- One program and its arguments, stored as a JSON argv array (`["launchctl",
  "print", "gui/501/com.example.job"]`). It is executed directly, never
  through a shell: no pipes, redirects, `;`, `&&`, globbing, `$()` or
  backticks. A check that needs a pipeline is a script in a repo, reviewed
  as code, and the check runs that script by path.
- It reads, it does not write. kit cannot prove that, so it refuses known
  writers outright: `sudo`, `rm`, `mv`, `cp`, `kill`, `pkill`, `launchctl`
  verbs other than `print` and `list`, `systemctl` verbs other than `status`
  and `is-active`, `docker` verbs other than `ps`, `inspect` and `version`,
  `git` verbs other than `log`, `show`, `grep` and `rev-parse`, `gh` other
  than `api` GETs and `view`/`list`/`checks`, `plutil` without `-o -`,
  `curl` or `wget` with a method, body or output file, and any argument
  that is a path into the dispatch, secrets or kit state directories.
- It never reads a secret: `op`, `hev secrets` and `security` are refused,
  and the environment is stripped to `PATH`, `HOME` and `LANG`. No Layer
  key, token file or service-account variable is passed.
- Success is exit status 0 and, when `check_expect` is set, a match of that
  regular expression on stdout.

**Review before keep.** A check is proposed with its claim (or added later)
and shown in the triage row as the argv, the host, the timeout and the
expected match, with any denylist hit in red. Keeping the claim does not
approve the check; the reviewer approves it separately, which copies
`check_hash` into `check_reviewed_hash` and writes a `check-approve` event.
The job runs a check only when the two hashes match, the claim is `active`
with `disposition: keep`, and the host's `board.recheck` setting is on.
Proposed, fix, dropped and superseded claims never run checks.

**Where it runs.** On `check_host`, through that host's hevd, as hevd's own
user, in an empty temporary directory that is removed afterwards. A check
with `check_host: repo` (c:436.1 reads `.github/workflows/ci.yml`) runs on
the consolidation host against a fresh read-only clone of the repo's
default branch. A host that is offline simply does not run its checks; that
counts as `error`, not `fail`.

**When.** At `recheck_at`, or two days before `valid_until` for host-state
claims, at most once a day per claim, with random jitter of up to an hour,
and at most 50 checks per host a day.

**Timeout and output.** `check_timeout` defaults to 10 seconds and is capped
at 60; on timeout the whole process group is killed and the result is
`timeout`. stdout and stderr are read up to 64 KB and the rest discarded.
The event keeps the exit status and the last 1 KB of output after
redaction (anything shaped like a token, key or password, and lines that
match the host's secret names, is masked). Output never goes into the
claim's sentence or a model page and is never fed to a model as
instructions; triage sees it as quoted text.

**Results.**

| result | effect |
|---|---|
| `pass` | a confirmation: `confirm_count` + 1, `last_seen` = now, dates move forward; `check_failures` and `check_errors` reset |
| `fail` (ran, wrong exit or no match) | first time: a `check` event and a retry the next day. Second consecutive time: the claim **expires** (`valid_until` = now, `state: expired`), lands in the queue's Recheck lane with the output, and every model citing it gets a change. Nothing is deleted or superseded: a failing check means "no longer known true", not "known false". The job keeps running the check once a day for 14 days; a pass in that time returns the claim to `active` |
| `error`, `timeout` (could not run) | not evidence about the claim. Three in a row suspend the check (`check_reviewed_hash` cleared, `check-suspend` event) and put the claim in the Recheck lane as "check broken"; the claim then expires by its own date as if it had no check |

**A changed check returns to triage.** Any write that changes `check`,
`check_host`, `check_expect` or `check_timeout` changes `check_hash`, so it
no longer matches the reviewed hash and the job stops running it at once.
The claim itself stays kept and active; it reappears in the Proposed lane
marked "check changed", showing the old and new check side by side, until
a reviewer approves the new one. A confirmation that arrives with a
different check (a session proposing the same fact with its own command) is
handled the same way.

**Guards, together.** Separate approval of the exact command, bound by
hash; kept claims only; argv without a shell; the denylist; no secrets and a
stripped environment; a per-host budget and timeout; output only in events,
redacted; a per-host off switch (`board.recheck = false`); every run an
event; and Adam's weekly audit lists every check approved that week,
whoever approved it.

### Background consolidation: by accumulated change, not a timer

Each claim event adds the claim's `importance` to `change_score` on every
model that cites the claim or whose `scope` shares an entity with it. A
model is rewritten when:

- `change_score` reaches 10 (about two important changes, or three
  ordinary ones), or
- a `decision` claim it cites is superseded (at once), or
- the session-start brief finds it has expired claims.

The rewrite runs on one designated host, the same way the secrets quota
meter is polled from one host (#467 post 3). The hevd cluster (RFC 0016, on
the 0.7 train) can elect it later. When five or more active claims share an
entity that no model's scope covers, the job proposes a new standing
question as a `proposed` model. Adam approves each new model (decision 8);
until then it is not rewritten, injected or returned by `recall`.

### Which harnesses can run them

| harness | session start | session end | what kit provides |
|---|---|---|---|
| Claude Code | `SessionStart` hook in settings; stdout goes into context | `SessionEnd` hook | the two commands above |
| Codex | `SessionStart` in `~/.codex/hooks.json`, already used by peers (`docs/peers-parity.md`) | a stop hook, where the installed Codex version has one | the same commands; otherwise hevd's fallback |
| pi | no shell hook from kit; the pi extension is not shipped from kit (`docs/peers-native-parity.md`) | none | `hev peers runtime` already wraps the harness launch and can print the brief into the first prompt; hevd's fallback for the end |
| any dispatched worker | `hev dispatch` adds the brief to the order's context, harness-independent | run completion | dispatch queues `propose` when the run ends |

**hevd's fallback.** hevd captures transcripts from all three harnesses. When
a captured session has been idle for 30 minutes and no end hook has run
since the last watermark, hevd queues `propose` itself. This is what makes
session-end proposals work for every session, on every harness, from the
start, including the FOH sessions that are never formally ended.

## 6. The first three mental models

Written by hand from today's board, to the shape of memo #467: facts, then
what to do, then what is in flight, then what is open. Every line cites
claims; each claim's `supports` is the thread in its id. Host addresses,
vault paths and service configuration are left out of this public copy, by
Adam's rule in #451; the live pages on the board carry them.

### `m:mini-live`: What is live on the mini that I shouldn't disturb?

Scope `host:mini`. Budget 600 tokens. Built from claims last seen between
2026-10-08 and 2026-11-15.

**Leave these running**

- Two Chrome instances with remote debugging are the X promo loops'
  browsers, not orphans. If they close or the mini reboots, X posting stops
  silently with exit 0, and the loop's sandbox cannot restart them. A third
  is a seat-check task's. [c:482.1, c:482.2, c:482.3]
- The kit docs preview is two launchd agents that serve and rebuild every 5
  minutes from the 0.7 train docs branch. It replaced the ad-hoc job and the
  older tmux preview. [c:458.2 supersedes c:458.1]
- A keep-awake agent keeps the console unlocked. Don't remove it. A reboot
  still stops at the login window and needs Adam. [c:227.1]
- hevd runs the `hev` on PATH directly, on the edge channel with hourly
  auto-update, and is a cluster member with Oven, the laptop and win-oven.
  [c:464.3, c:470.4, c:470.7]
- The kit dashboard service tracks edge. [c:459.2]
- hev dot 0.4.0 runs in a tmux session, with live acts behind a spoken yes,
  and can dispatch to win-oven. [c:472.1, c:475.1 supersedes c:472.3]
- hev media edits run as dispatch runs on the mini (`edit-<job>-r<N>`).
  [c:481.1]
- Scheduled tasks kept on 2026-11-13: the promo loop and the nightly offsite
  backup. Six agent tasks were retired, reversibly. [c:469.4, c:454.1]
- The media stack's timer is paused; its containers are up but will not come
  back by themselves after a crash or reboot. [c:466.4]

**Don't**

- Read a plist with `plutil -extract KEY json` without `-o -`: it overwrites
  the file. Seven launchd jobs have no source outside the LaunchAgents
  folder. [c:469.1, c:469.2]
- Restart a secret-dependent service while the 1Password quota is spent; see
  `m:secrets`. [c:467.3]
- Report GUI permissions as off when the console is locked. [c:226.1]
- Edit the mini's dotfiles in place; they link into hev/shell. [c:451.2]

**Open problem**

- The mini takes no GitHub Actions jobs: its runner containers crash-loop on
  a deprecated runner version. [c:422.2, fix]

**Check live, the page can't know:** `peers`, `hev dispatch ls --host mini`,
`hev cluster ls`.

**Could not answer from the board:** which of the mini's roughly 44 launchd
agents are loaded now and who owns each (#467 gives only the count); which
agent sessions are running (peers knows, the board doesn't); whether the
CRM, intranet and snapshot services are up (#451 says only where their
plists live); whether the seat-check Chrome is still wanted; Docker and
Colima state. Two of the cited claims (#226, #227) are older than 14 days
and would arrive expired; most of the page expires by 2026-11-29 unless
rechecked. That is the intended behaviour: this question has no
long-lived answer.

### `m:secrets`: How do secrets and 1Password work here, and what breaks?

Scope `area:secrets`, `tool:1password`, all hosts. Budget 600 tokens.

**Facts**

- One 1Password daily request allowance is shared by every service account
  and machine on the account; this account's is 1,000 a day. When it is
  spent, anything that reads a secret at start fails until the reset.
  [c:467.1]
- Archived items keep their titles. `op inject` (which kit's cache uses) can
  resolve a title to an archived duplicate where `op read` finds the live
  one. [c:483.3]

**Rules**

- Nothing calls `op` directly. Every read goes through `hev secrets` (`run`,
  `read`, `env`). [c:467.2]
- A polling worker or wrapper resolves once and holds the value; it never
  fetches per launch or per poll. [c:466.2]
- No plan upgrade: consolidate scripts onto kit, one secrets daemon per
  host. [c:467.4]
- Don't restart a secret-dependent service while the quota is spent; capture
  daemons read their Layer key at start. [c:467.3, c:464.4]

**How to apply**

- All four hosts run the same budget policy: per host 200 a day and 40 an
  hour, per caller 5 a minute, account 900 a day, which leaves 100 for direct
  `op` calls. [c:470.8]
- After the service-account token file is rewritten, restart hevd on every
  host; a daemon started earlier keeps the old token and `hev secrets ping`
  exits 69. [c:467.6]
- When a `hev secrets` read fails, reproduce with `op inject` to see op's
  real error; kit currently reports every non-rate-limit inject failure as
  "service account required". Reference items by id. [c:483.1, c:483.2]
- During an outage, the attended fallback on the laptop works with Adam's
  fingerprint. [c:467.7]
- Old vault-path threads are history: #49's path never resolved, #50 gave the
  working one, and config now resolves names through `secret://NAME`.
  [c:50.1 supersedes c:49.1; c:467.8]

**Open problems**

- Two tools on the mini still call `op` with an inherited token, so the
  mini's shell keeps exporting it as a marked local edit. [c:467.9, fix]
- win-oven's credential wrapper still makes three 1Password reads per
  launch. [c:470.6, fix]
- The rotation restart and the misleading error are kit fixes in flight
  (orders `kit-layer-key-config-20261115`, `kit-secrets-op-error-20261115`).
  [c:467.6, c:483.2, fix]

**History.** The quota ran out on 2026-10-20 (#336), on 2026-11-13 (#466,
which led to memo #467), and the cache went dark again twice after the memo
(#467 post 4, #483). This page is the memo turned into something injected at
session start, rather than something an agent has to find.

**Could not answer from the board:** today's quota use (`hev secrets status`
knows); whether the two kit orders above have merged and been installed;
whether win-oven's wrapper was converted after 2026-11-14; who owns token
rotation; whether the 100-request headroom is still needed once the mini's
two direct callers are converted.

### `m:worker-finish`: How does a dispatched worker finish its run?

Scope `area:dispatch`, all repos. Budget 500 tokens.

**The rule**

- Workers push their branch and open the PR; FOH merges. (Adam, 2026-11-15;
  replaces "stop at a local commit".) [c:443.4 supersedes c:443.1]
- Deploys, tags and releases wait for Adam's own go, which FOH holds.
  Workers don't accept merge or deploy authorization relayed by a peer.
  [c:443.2, c:443.6]
- FOH merges hev/kit PRs as soon as independent review and verify report
  ready, then starts the lane's next order. [c:443.3]
- Exception, hev media: a worker may merge its own PRs and deploy to the
  mini's media service, after a DB backup before server upgrades and with a
  real browser check after the deploy. [c:443.5]

**Who launches what**

- FOH launches an order's first worker. The orchestrator may launch rework,
  retry and validation inside that order and reports each launch to FOH.
  New scope, new orders and outward messages go through FOH. [c:442.1]
- When that policy changes, update the orchestrator's brief on its host too,
  or it refuses the work. [c:442.2]
- Any step after a run finishes (review, merge on green) goes on a host with
  `hev expedite`, never as a wait held in a laptop session, because the
  laptop sleeps. [c:465.1]

**Known problems**

- `hev expedite` readiness and merge support GitHub only. [c:465.3]
- `hev expedite` resolved the reply address differently from peers
  (KIT-30). [c:465.2, fix]

**Superseded, kept as history:** #106's "merge it when green" and #191's
"workers may undraft and merge their own PR" are superseded by c:443.4.
#106's other half, "don't invent approval gates Adam didn't ask for", is a
separate decision claim and stays active. [c:106.1]

**Could not answer from the board:** the run-end contract itself (the last
line of `status.md`, `report.md`, `needs-decision`) is in the dispatch skill
and order templates, not on the board; whether layer-pro follows the same
merge rule (the inventory's conflict #106/#443 is unresolved); whether PRs
open as draft or ready by default; what "independent review and verify"
must include; what a worker should do when CI cannot run at all (#468's
queued jobs).

## 7. Triage UI

Two new areas in the existing web UI, plus facet filters on the board pages
(section 3). The board pages (index, boards, threads, search, new) stay the
home for episodes.

**Queue (`/queue`).** One list, five lanes as tabs with counts: *Proposed*
(including "check changed"), *Conflicts*, *Recheck* (expired host-state,
overdue external claims, failed and broken checks), *To-dos* and *Client*.
Each row shows:

- the sentence, editable in place;
- provenance, `about` chips, importance, gate score and the session class it
  came from (worker, FOH, machine);
- evidence: the supporting thread titles and quoted post lines, or for a
  trace-backed claim the quoted turns with the speaker (Adam, the agent, a
  peer);
- the five nearest active claims with their labels from the contradiction
  check;
- the check, if any: argv, host, timeout, expected match, denylist hits, and
  the last result with its output; a separate **Approve check** button;
- buttons **Keep**, **Fix**, **Drop** and **Supersedes...** (pick the claim
  it replaces), and **Widen** on client claims; keyboard `k`, `f`, `d`, `s`.

The *Client* lane holds `visibility: client` claims. It is shown to Adam and
to sessions running in that client's folder; a FOH session's queue view
leaves it out (decision 9). The header line shows today's keep-rate and the
last 30 days', proposals per session class, deferrals under the cap and the
age of the oldest untriaged proposal. Bulk actions apply to a selection, so
the seed migration's 213 claims can be worked through in sittings.

**Audit (`/queue/audit`).** Adam's weekly page (decision 2): 25 triage events
sampled from the week, stratified so each session class and each action
appears, plus every check approved that week and every widened client claim.
Each item has **Agree** and **Overturn**; the agreement rate is kept as a
metric.

**Models (`/m/`, `/m/{id}`).** A wiki: an index of standing questions grouped
by scope, then one page per model with the answer, each citation linking to
`/claim/{id}`; the "could not answer" list; the last rewrite time and what
changed since. Proposed models are listed apart with an **Approve** button
that only Adam's identity can use (decision 8). An *Ask a new question* form
proposes a model.

**Claim page (`/claim/{id}`).** The sentence, state, dates, edges drawn as a
short chain (superseded by, supersedes, caused by), the supporting posts and
traces, the check and its run history, the models that cite it, and its
event history. (`/c/` is already taken by canvas pages, so claims use
`/claim/`.)

Threads link the other way: a thread page lists the claims it supports under
its title, so a person reading an episode sees what was kept from it, and
shows its facets as chips (section 3).

## 8. Evaluation

`hev board eval FIXTURE.json` (PR #245, landed as a draft) scores a private
fixture against the live board through the search endpoint: recall@k,
`stale_as_current`, `superseded_surfaced`, abstention precision,
`correct_and_current`, mean and p90 tokens, and recall broken down by
question type and provenance. Its sources are thread ids.

To score this design it needs one addition: `--target memory`, which runs
`recall` instead of `search` and counts a claim as a hit when its `supports`
include a source thread, and a model as a hit when it cites such a claim.
Thread-level and memory-level runs on the same fixture are then directly
comparable.

**Baseline (the runner, PR #245):** recall 21/21, correct-and-current
14/27, stale-as-current 4, abstention 0/6. The fixture has 27 items, 21 with
a source thread and 6 that should abstain; correct-and-current counts all
27. For reference, the hand judgement of 2026-11-15 on its 21 questions gave
recall 19/21 and 12/19 correct and current; the triage sample's keep-rate
was 37%. Recall is not the gap; currency and abstention are.

| feature | metric | target |
|---|---|---|
| facets on threads (decision 10) | recall@k with the search unchanged; facet coverage (threads with at least one facet); facet-filter recall (fixture items whose source thread survives a `ls` filtered by the item's repo or host) | 21/21 unchanged; every thread but the one with none; at least 95% |
| expiry by provenance | `stale_as_current` | 0 (from 4) |
| supersedes and the contradiction check | `superseded_surfaced`; `correct_and_current` | 0; at least 22/27 at the seed gate, 24/27 after models |
| `fix` leaves memory | hits judged `fix` that are returned as current knowledge | 0 (returned only labelled as open problems) |
| claims as the unit | recall@5; mean and p90 tokens | no lower than 21/21; tokens at least halved |
| recall's abstention | abstention precision | at least 4/6, with no new false abstentions on the 21 |
| models | synthesis recall (questions of type `synthesis`) | the mini question answered from `m:mini-live` |
| surprise gate, every session (decision 7) | keep-rate on proposals, overall and per session class; proposals per session; age of the oldest untriaged proposal | 30 to 45% in every class; near 1 for workers and FOH; under 3 days |
| triage by FOH (decision 2) | Adam's agreement rate on the weekly audit | at least 90% |
| recheck (decision 6) | share of host-state claims in models past `valid_until`; checks run without a matching reviewed hash (from events); check `error` rate | under 10%; 0, always; under 5% |
| session-start brief | repeat incidents of a problem that has an active claim, counted from traces | down from the two recurrences after #467 |
| `search` default (decision 4) | `--target memory` against `--target search` on the same fixture | recall no worse (21/21) and stale-as-current no higher before the switch |

Every stage below names the metric that gates it. A change that moves its
own metric and lowers recall@5 does not ship.

## 9. Migration plan

| stage | work | gate |
|---|---|---|
| 0. Baseline | done: the eval runner (PR #245, draft) and the fixture; numbers above | runner reproducible (two identical runs) |
| 1. Facets on threads | board schema adds the six facet attributes; `body` and `title` declared non-filterable; backfill from `inventory.json`; tag-map and author facets for #474 onward and new posts; `ls`/`search`/API filters; UI chips, filter bar and review page | eval unchanged (recall 21/21, nothing else moves); facet coverage and facet-filter recall targets; wire fixtures pass |
| 2. Pilot | memory namespace (decision 1), claim/model/event rows, `get`/`describe`/`apply`, `hev get todos` (decision 3), `recall` beside `search` (decision 4) behind `board.memory = true`; the 20 pilot claims below and the three models above entered by hand, the models approved by Adam | `--target memory` on the fixture's subset: synthesis 1/1, stale-as-current 0 for the pilot's subjects |
| 3. Session end for every session | `propose` on end hooks, dispatch completion, hevd's idle fallback and the machine-session batches, all at once (decision 7); the triage queue and the weekly audit (decision 2); caps; check proposals and approvals in triage, but no runner yet | keep-rate 30 to 45% per session class; oldest untriaged under 3 days; audit agreement at least 90% |
| 4. Seed | the 213 seed claims plus threads since the snapshot, all `proposed`, worked through beside the daily queue; conflicts resolved | stale-as-current 0 and correct-and-current at least 22/27 on the full fixture |
| 5. Recheck | the recheck job on for kept claims with approved checks (decision 6), denylist and redaction in place, per-host switch on | expired share in models under 10%; zero runs without a reviewed hash; error rate under 5% |
| 6. Brief, consolidation, default | session-start brief for Claude Code and Codex, dispatch context for all; model rewrites on change and model proposals to Adam (decision 8); `search` switches to `recall` for agents only when its gate passes (decision 4) | `correct_and_current` at least 24/27; abstention at least 4/6; tokens halved; the `search` default gate in section 8 |

Session-end proposals move ahead of the seed (they were last in the first
draft) because decision 7 turns them on for everything at once: new
knowledge stops arriving untriaged before the backlog is worked. The
recheck job follows the seed so that the host-state claims it keeps alive
have been triaged and their checks reviewed first.

### The pilot: the 11 kept threads as claims

Adam kept #72, #73, #92, #122, #134, #142, #145, #236, #249, #371 and #436
from the triage sample. Each yields at least one claim. Where a thread holds
more than one fact, the extra claims show how triage splits it. Dates are
`valid_from`; "recheck" is `recheck_at`; host-state claims show
`valid_until`.

| id | sentence | provenance | disposition | about | validity |
|---|---|---|---|---|---|
| c:72.1 | On layer-pro, CI for a draft PR skips the Rust, lock-convoy, gateway-mirror and pgvector jobs and still shows green, so mark the PR ready or dispatch `ci.yml` on the branch before calling CI green. | procedure | keep | repo:layer-pro, tool:github-actions, area:ci | from 2026-09-28; confirmed 4 times, last 2026-11-03; recheck 2027-05-02 |
| c:72.2 | After `gh pr ready`, wait for the ready-triggered run to classify before dispatching a manual run, and never cancel the PR's own run because a manual run is on the same head. | procedure | keep | repo:layer-pro, tool:github-actions, area:ci | from 2026-09-29; recheck 2027-04-13 |
| c:72.3 | Turbopuffer write responses report `rows_upserted`, `rows_patched` and `rows_deleted` beside `rows_affected`, and omit any count that is zero. | external | keep | tool:turbopuffer | from 2026-09-28; recheck 2026-12-27 |
| c:73.1 | A Layer CE gateway's telemetry can be captured without PostHog by pointing `LAYER_TELEMETRY_ENDPOINT` at a throwaway HTTP listener; `gateway_started` arrives within seconds as plain JSON. | procedure | keep | repo:layer-pro, area:telemetry | from 2026-09-28; recheck 2027-03-27 |
| c:73.2 | bosshogg is not installed on the mini, so PostHog queries run from the laptop. | host-state | drop | host:mini, cli:bosshogg | from 2026-09-28; valid until 2026-10-12, expired |
| c:92.1 | For an arbitrary-JSON field in a Kubernetes CRD, emit `type: object` with `x-kubernetes-preserve-unknown-fields: true` (schemars `schema_with`), because deriving JsonSchema for `serde_json::Value` emits `{}`, which is not a structural schema. | external | keep | repo:layer-pro, tool:kubernetes, area:ingest; `visibility: shared` although the episode is client work | from 2026-09-29; recheck 2026-12-28 |
| c:122.1 | In Chromium, calling native `fetch` as a method of another object throws "Illegal invocation", so wrap it as `(...a) => fetch(...a)`; Node fixture tests do not catch this. | external | keep | tool:playwright, area:site, area:tests | from 2026-10-01; recheck 2026-12-30 |
| c:134.1 | On turbopuffer-backed Layer, an attribute can be made filterable with a schema-only write and no row rewrite; a large namespace took about five minutes before filtered queries succeeded, and FTS plus filterable bills the attribute at 200%. | external | keep | repo:kit, tool:turbopuffer, area:search, area:traces | from 2026-10-02; recheck 2026-12-31 |
| c:134.2 | Python `urllib`'s default User-Agent gets a Cloudflare 403 (error 1010) on our hosted sites, so set a User-Agent. | external | keep | tool:cloudflare, tool:python | from 2026-09-29 (#88); supports 10 threads (#88, #124, #134, #144, #145, #167, #203, #218, #222, #405), confirm_count 10 |
| c:142.1 | kit stores instruction-file versions as metadata rows in `<archive>-instructions` and their searchable text as ordinary chunks with `harness=instructions`; each version id chains to the previous one. | procedure | keep | repo:kit, area:traces | from 2026-10-02; recheck 2027-03-31 |
| c:142.2 | A Layer query that sends `include_attributes=true` together with `exclude_attributes` is rejected with HTTP 400, so send `exclude_attributes` alone. | external | keep | repo:kit, tool:turbopuffer, area:search | from 2026-10-02; recheck 2026-12-31 |
| c:145.1 | The public edge masks Layer gateway JSON errors as "error code: 502"; port-forward the gateway to read the real error, and read its pod logs with `-c gateway` because the default container is postgres. | procedure | keep | repo:layer-pro, tool:kubernetes, area:search | from 2026-10-03; recheck 2027-04-01. The thread is `resolved`; this side claim is not |
| c:236.1 | On Colima, `docker run -v /tmp/...` mounts come up empty, so mount a path under `$HOME`. | external | keep | tool:colima, tool:docker | from 2026-10-12; recheck 2027-01-10. The episode is superseded client work (a client repo); this claim is `shared` and active |
| c:236.2 | `gh pr merge` fails while the GitHub GraphQL quota is exhausted; merge through the REST API instead. | external | keep | tool:github, cli:gh | from 2026-10-12; recheck 2027-01-10 |
| c:249.1 | On layer-prod the metrics store is a sidecar in the gateway pod with no Service, so port-forward the pod; the runbook's Service name does not exist. | host-state | keep | host:layer-prod, repo:layer-pro, tool:kubernetes | from 2026-10-13; confirmed 2026-10-14; `valid_until` set to 90 days by triage as slow-moving topology: 2027-01-12 |
| c:249.2 | PromQL `increase()` reads 0 for a series that first appears inside the window, so snapshot the raw counter at the start and end instead. | external | keep | tool:kubernetes, area:telemetry (the closed tool list has no metrics-store value) | from 2026-10-13; recheck 2027-01-11 |
| c:249.3 | The layer-pro runbook for the lookup UAT names a metrics Service that does not exist and masks a failed fetch as empty counters. | procedure | **fix** | repo:layer-pro, area:telemetry; in `hev get todos`; `todo` empty until someone takes it | from 2026-10-13; leaves models; returned only as an open problem |
| c:371.1 | hev search's Lance `/query`, `/list` and `/facet` predicates need backtick column identifiers (a double-quoted column is a string literal and silently matches nothing), while `/aggregate` uses DataFusion SQL and needs double quotes. | external | keep | repo:kit, repo:layer-pro, area:search | from 2026-10-30; recheck 2027-01-28 |
| c:371.2 | For a metadata-first write to hev search, declare the complete native schema before the first upsert; an empty schema followed by an upsert that adds strings or lists fails. | external | keep | repo:layer-pro, area:search | from 2026-10-30; recheck 2027-01-28 |
| c:436.1 | kit's `windows-inventory` CI job runs the `internal/nodes` tests and then the sender test in one step without checking the first exit code, so failing nodes tests are hidden behind a green job. | procedure | **fix** | repo:kit, tool:github-actions, area:ci, area:tests; in `hev get todos`; `todo` empty until someone takes it | from 2026-11-06; confirmed 2026-11-15 by reading `.github/workflows/ci.yml:213` on main (still unchecked) |

What the pilot shows:

- **Eleven threads, 20 claims, two of them fix to-dos, one dropped.** The
  thread is the wrong unit in both directions: #72 holds three claims, and
  the one thread with no keepable head (#236) holds two shared lessons.
- **Rule-based seeding alone keeps 9 of the 11.** #145 (`resolved`) and #236
  (`superseded`) produce no active seed claim under section 3's rules; their
  kept lessons only appear when triage lifts side claims. Triage of resolved
  and superseded threads cannot be skipped.
- **Confirmation replaces duplication.** c:134.2 absorbs ten threads that
  each rediscovered the same Cloudflare block. #142's `gh pr checks` EOF note
  becomes a confirmation of the existing canonical claim from #150, not a
  new claim.
- **Reconfirmation is cheap when the claim is checkable.** c:436.1 was
  rechecked against the repo while writing this RFC; it still holds.
- **Few kept claims are checkable unattended.** Of the 17 kept claims, only
  the host-state c:249.1 is a natural check candidate, and it needs cluster
  credentials the stripped check environment does not pass, so it relies on
  triage's 90-day date instead. External and procedure facts are rarely
  verifiable by a cheap read-only command. Automatic rechecks will matter
  mostly for the host-state claims behind `m:mini-live` (a launchd job
  loaded, a process listening, a tmux session present).

## 10. Decisions (2026-11-16)

Adam answered the first draft's ten open decisions on 2026-11-16 (memo
#492). They are folded into the sections above as design.

| # | decision | where |
|---|---|---|
| 1 | Claims, models and events live in a second Layer namespace, `<board>-memory`; threads untouched. | section 2 |
| 2 | FOH sessions triage proposed claims; Adam audits a weekly sample. | sections 4, 7 |
| 3 | `fix` to-dos are a list in kit, `hev get todos`, not Linear. | section 2 |
| 4 | `hev board search` keeps returning threads; `recall` is added beside it; the default switches only after the eval shows recall is no worse. | sections 4, 8, 9 |
| 5 | Host-state claims expire 14 days after last seen; triage may set a longer date for slow-moving facts. | section 2 |
| 6 | Automatic rechecks are on, for kept claims only, with the check command reviewed at triage. *Differs from the first draft (off).* | sections 2, 5, 9 |
| 7 | Session-end proposals run for every session from the start, workers and FOH. *Differs from the first draft (workers first).* | sections 5, 8, 9 |
| 8 | Adam approves each new model page. | sections 2, 4, 5, 7 |
| 9 | Client claims are visible only in that client's sessions until triage widens them. | sections 2, 5, 7 |
| 10 | The five #474 facets are written onto existing thread rows now. *Differs from the first draft (left open).* | sections 2, 3, 7, 9 |

### New open questions

These follow from decisions 6, 7 and 10:

1. **Machine sessions:** do scheduled loop iterations and expediter rounds
   count as "every session" (this design batches them per loop per day), or
   are they excluded?
2. **Who approves a check:** any FOH triage session, with Adam seeing every
   approval in the weekly audit (this design), or Adam before a check first
   runs?
3. **Who triages client claims:** Adam in the UI, or a session running in
   that client's folder, since FOH sessions can't see them?
4. **Trace-backed claims:** may a claim rest on a trace alone (this design),
   or must the session-end job post a thread so a person can read the
   evidence on the board?
5. **Facets at post time:** should `hev board post` refuse a thread with no
   repo facet outside `floor`, or keep inferring and listing gaps for
   review (this design)?
6. **Caps:** 40 proposals per host a day and a 150-item queue ceiling, or
   different numbers once stage 3 has a week of data?
