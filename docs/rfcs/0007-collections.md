# RFC 0007: Collections — one `hev query` over traces and what was kept from them

Status: Draft (2026-09-26). Groundwork landed with this RFC: `pkg/search`.

## What changes for you

You ask one question and get back both what happened and what someone decided
was worth keeping:

```bash
hev query "why does the 5432 service container collide" --in traces,board
hev query --in board --since 14d "release train"      # only the curated notes
```

Today the raw record (`hev query`, over trace chunks) and the curated record
(a board of agents' notes, in its own Layer namespace) are two tools with two
syntaxes, and agents use neither consistently. After this, they are two
collections in one store, searched by one command with one set of flags.

## Motivation

- **Traces are the raw record.** Every turn of every session, searchable a
  few seconds after it is written. They answer "what did the agent try", but
  a finding is spread across forty turns next to the dead ends.
- **A curated collection is the distillate.** A note that says "the port-5432
  container collides when two CI jobs start together; rerun, don't chase" is
  what the next session needs, in one hit. hev factory's board is the first
  such collection. It is already a Layer namespace, embedded with kit's model
  and searched with kit's hybrid query.
- **Two front doors means agents learn neither.** One command that reaches
  both is something an agent's instructions can name once.

## Decision

1. **The query is a library.** `pkg/search` builds kit's request for any
   namespace: an ANN and a BM25 leg per phrasing (up to 8), fused by RRF in
   the store, with `And`, `ParseSince` and `Since` for scoping. `hev query`
   uses it, and so does any tool that searches a Layer namespace the way kit
   does. That is how a board search and a trace search rank the same way.
   *Landed.*
2. **A collection is configuration, not code.** kit learns no product's
   schema. A collection names a namespace, its text column, the attributes a
   hit shows and which attribute is its time:

   ```toml
   [collections.board]
   namespace = "factory-board"
   time      = "created"            # RFC 3339-ish text; compared as text
   show      = ["board", "title", "author", "thread_id"]
   link      = "http://echo:7700/t/{thread_id}"
   ```

   `traces` is built in and is the default, so `hev query` without `--in` is
   what it is today.
3. **`--in a,b` queries each collection in parallel and interleaves by rank.**
   Every collection is fused inside the store. Two fused lists are merged by
   RRF again on the client, the one fusion kit does itself: collections live
   in different namespaces, and a multi-query cannot span namespaces. `--top`
   applies after the merge. `--json` adds `collection` to each hit, alongside
   that collection's `show` attributes.
4. **Shared flags mean the same thing everywhere.** `--also`, `--since`,
   `--top` and `--json` apply to every collection. A collection-specific
   filter is written `--where board=tooling`, and is dropped with a warning
   for a collection that lacks the attribute. `--plan`, `--workdir` and
   `--harness` remain shorthand for `--where` on `traces`.

## Non-goals

- Writing to a collection. `hev query` reads. The board keeps its own CLI and
  API for posting, and distillation from traces into the board is a factory
  loop, not kit.
- One namespace for everything. Traces and notes have different schemas and
  lifetimes, and one namespace has one schema.
- Cross-collection reranking by a model. RRF over RRF is enough until it
  demonstrably isn't.

## Proposed surface

```bash
hev query [--in traces,board] [--also …]… [--since 7d] [--top 10] [--where k=v]… [--json] QUERY
hev collections                      # configured collections, and whether each namespace answers
```

## Open questions

- Should a collection declare its own credentials (a board on another Layer
  account), or share the `[layer]` table? Shared for now.
- Is the board's `time` better as an epoch attribute, so `--since` is a
  numeric filter? Text works because the board writes a fixed-width UTC
  layout. A new collection should use RFC 3339.
- Does the dashboard get an "in" selector, or does the factory UI own the
  mixed view? The factory UI reads the board through its own API today.
