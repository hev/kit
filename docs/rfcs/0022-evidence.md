# RFC 0022: Evidence — multimodal proof packages, folders in OSS, EmbeddingGemma 2 retrieval and federated search in Pro

**Status: Draft (2026-11-20).** OpenAPI contract: [`openapi/evidence-v1.yaml`](../../openapi/evidence-v1.yaml) (OpenAPI 3.1.0, additive to `openapi/kit-v1.yaml` from PR #298). Builds on RFC 0019 (kit Pro API `/v1` in PR #298, board memo #534), [RFC 0019 (Gates)](0019-gates.md), [RFC 0007 (Collections)](0007-collections.md), and board memo #551 (OSS vs Pro UI split).

> Numbering: `0020` is the harness registry on `v0.7` and `0021` is taken by Scorers in PR #284, so this is `0022`.

## Summary

**Evidence** is kit's sixth data surface, alongside **Traces**, **Board**, **Orders**, **Peers**, and **Hooks**. It captures unstructured, **multimodal** artifacts — **text**, **image**, and **video** — produced whenever an assignment calls for it, primarily by validators, browser-use runs, computer-use runs, and spikes.

Evidence follows the exact OSS vs Pro seam defined in board memo #534 and the Pro API RFC — with **all six OSS surfaces unified under `~/.hev/<surface>/`** for a 1-to-1 free-to-paid mapping, and **Pro hosts staying off local disk** to save host storage:

| Surface | OSS folder (`~/.hev/`) | Pro API (`/v1/`) |
|---------|------------------------|------------------|
| Traces | `~/.hev/traces/` | `/v1/traces/*` |
| Board | `~/.hev/board/` | `/v1/board/*` |
| Orders | `~/.hev/orders/` | `/v1/orders/*` |
| Peers | `~/.hev/peers/` | `/v1/peers/*` |
| Hooks | `~/.hev/hooks/` | `/v1/hooks/*` + webhooks |
| **Evidence** | **`~/.hev/evidence/`** | **`/v1/evidence/*`** |

- **OSS:** A plain folder of self-contained packages on disk (`~/.hev/evidence/<id>/manifest.json` plus the raw files beside it). No daemon, no local model, no API server, and **no tab in OSS `hev serve`** (which stays Traces + Board per memo #551). The CLI supports writing, inspecting, exporting, and lexical grep over manifests and text items.
- **Pro:** Hosts write and read strictly through `/v1/evidence/*` (Layer + Pro object storage) and **do not persist packages or media blobs on local disk** — saving disk space on cluster hosts, especially for video. Pro provides two first-class hosted retrieval capabilities plus a first-class viewer in the Pro UI:
  1. **`/v1/evidence/*`** — package CRUD, content-addressed media blobs (with streaming/presigned upload for video), and unified multimodal search backed by **`google/embeddinggemma-2`** in Layer.
  2. **`/v1/search`** — a **federated search API** across kit's searchable surfaces (`traces`, `board`, `orders`, `evidence`), supporting both text (`GET`) and multimodal (`POST`) queries fused via RRF in Layer.
  3. **Pro UI Evidence viewer** — first-class gallery, lightbox, HTML5 video player, text/diff viewer, optional claims badges, and inline links on Order and Trace detail pages.

## Motivation

- **Validation and visual work produce multimodal proof that has no home.** Browser-use sessions, computer-use runs, visual UAT, and research spikes produce screenshots, screen-recording videos, console dumps, HAR/JSON receipts, and diffs. Today those scatter into ad-hoc folders (`docs/evidence/`, run worktrees, external drives) with no standard schema, no provenance links, and no search.
- **Traces and Board are the wrong abstractions for proof packages.**
  - **Traces** capture the raw turn-by-turn session stream; transcript screenshots are incidental to tool results and tied to one session's blocks.
  - **Board** is collective memory for curated prose findings and decisions, not a high-volume multimodal archive of browser videos and validation runs.
  - Keeping Evidence as a **distinct 6th category** leaves Traces and Board focused while letting Orders, Gates, Board posts, and Traces cite packages cleanly by `evidence_id`.
- **Packaging artifacts improves retrieval.** A lone screenshot or 15-second screen recording is ambiguous without context. Grouping related artifacts into an **Evidence Package** (with title, summary, provenance, and optional claims) lets Layer embed every text, image, and video item in a shared vector space while ranking and returning coherent packages — the same grouping pattern that already works for Board threads and Trace sessions.
- **Multimodal embeddings just crossed the practicality threshold.** `google/embeddinggemma-2` (Apache 2.0, modular 740M total — 270M text + 170M vision + 300M audio, ~440M with `audio_config=None`, 8K context, 768d Matryoshka vectors) natively maps text, code, images, and video into **one vector space**. That makes text-to-image, text-to-video, and image-to-evidence retrieval straightforward in Layer without chaining bespoke captioning pipelines.
- **Agents need one front door to retrieve what happened, what was decided, and what was proved.** Adding Evidence beside Traces and Board makes a **federated `/v1/search`** across all three immediately valuable — and natural on Layer.

## Decision

1. **Evidence is the 6th kit data surface.** Same storage-mode seam (`internal/storagemode`), same CLI + bundled skill (`hev evidence`, `hev-evidence`), same wire-equals-folder discipline. Trace images (`/v1/traces/images`) and Board blobs (`/v1/board/blobs`) stay independent; Evidence does not absorb them.
2. **The unit is an Evidence Package (`ev_<ulid>`), mostly neutral with optional claims.** A package holds ordered multimodal `items[]` (`text | image | video`), provenance (`order`, `session`, `repo`, `branch`, `commit`, `host`, `author`, `role`, `source`, `gate`, `tags`), and optional `claims[]` (`text`, optional `criterion`, optional `verdict`, optional `item_ids`). Packages can be written one-shot (`sealed`) or built incrementally (`open` → `add` → `claim` → `seal`) during a live browser-use or computer-use run.
3. **OSS is strictly a folder of files under `~/.hev/evidence/` (unified `~/.hev/<surface>/` rule).** All six OSS surfaces live under `~/.hev/{traces,board,orders,peers,hooks,evidence}/` so starting free and upgrading to Pro maps 1-to-1 to `/v1/*` (resolving Open Question #6 in PR #298). For Evidence, `~/.hev/evidence/<id>/` contains `manifest.json` and the raw item files right beside it. `hev evidence` writes, reads, exports, and runs lexical grep (`rg`/Go) over manifests and text items. OSS `hev serve` adds **no Evidence tab**.
4. **Pro stays off local disk and embeds with `google/embeddinggemma-2` in Layer.** In Pro mode, `hev evidence` uploads blobs straight to `/v1/evidence/blobs` (or staged presigned uploads for video) and reads strictly from `/v1/evidence/*` — **never duplicating packages or media files under `~/.hev/evidence/`**, keeping cluster hosts thin and saving disk space. Clients never compute or transmit raw vectors; the wire contract stays model-agnostic. Note: `google/embeddinggemma-300m` (EmbeddingGemma 1) is text-only (2K context); Layer targets **`google/embeddinggemma-2`** (with `audio_config=None` unless audio is added later) so text, images, and video share one 768d vector space.
5. **Video v1 is whole-clip with a hard cap, reserving segment fields.** In v1, clips are capped at `<= 60s` (`60,000 ms`) and `<= 100 MiB` and embedded whole within EmbeddingGemma 2's 8K context. Search hit matches reserve an optional `segment: {start_ms, end_ms}` object so time-sliced video indexing can ship later additively without breaking `/v1`.
6. **Federated search (`/v1/search`) ships alongside `/v1/evidence/*`.** Defaults to `in=traces,board,orders,evidence` (allowing design specs/RFCs merged into Orders to be discovered progressively alongside traces, board memory, and multimodal evidence). `GET /v1/search` takes text queries (`q` + up to 7 `also`); `POST /v1/search` adds optional multimodal query inputs (`media[]` — image or video) scored against `evidence` and fused with text legs via RRF in Layer.
7. **First-class viewer lives in the Pro UI.** Gallery, search (text + visual drop), inline lightbox, video player with HTTP Range scrubbing, text/diff viewer, claims badges, and deep links to/from Orders and Traces.

## Goals

- Define one stable `EvidencePackage` schema that is identical on disk (OSS) and over `/v1/evidence/*` (Pro).
- Support `text`, `image`, and `video` items with strict byte-sniffed validation, size/duration caps, and text redaction before storage.
- Support both one-shot capture (`hev evidence put`) and streaming capture (`create` → `add` → `claim` → `seal`) for browser-use and computer-use loops.
- Define Pro's `/v1/evidence/*` routes, presigned/chunked upload flow for larger videos, and package-grouped multimodal search.
- Define Pro's federated `/v1/search` endpoint over `traces`, `board`, and `evidence`.
- Define the `evidence-*` hook events and Pro UI viewer requirements.

## Non-goals

- **Ambient / continuous screen recording.** Evidence is explicit and assignment-driven. Always-on capture remains out of scope (see [RFC 0001](0001-frame-capture.md)).
- **Replacing Trace images or Board blobs.** Existing `/v1/traces/images/{sha256}` and `/v1/board/blobs/{sha256}` remain unchanged.
- **Audio modality in v1.** `google/embeddinggemma-2` supports audio, and the modality enum can grow additively later, but v1 scope is strictly `text | image | video`.
- **On-device / local multimodal embedding in OSS.** OSS stays zero-model, zero-Docker, plain files + lexical grep.
- **An Evidence tab in OSS `hev serve`.** OSS web UI remains Traces + Board only (memo #551).
- **Persisting prohibited client data.** Evidence respects host and client boundaries (board memo #445): it never auto-harvests external client systems.

## Vocabulary

| Term | Meaning |
|------|---------|
| **Evidence package** | One coherent collection of multimodal proof artifacts (`ev_<ulid>`), with title, summary, provenance, optional claims, and ordered items. |
| **Evidence item** | One typed artifact inside a package (`evi_<ulid>`): `text`, `image`, or `video`, with sniffed media metadata, sha256, and optional caption/context. |
| **Claim** | Optional assertion attached to a package (`text`, optional `criterion`, `verdict`, and `item_ids` pointing to supporting items). |
| **Package state** | `open` (accepting items/claims during an active run) or `sealed` (finalized immutable capture). |
| **Federated search** | One `/v1/search` request querying multiple kit surfaces (`traces`, `board`, `evidence`) in parallel on Layer and fusing results by RRF. |

## Data model and wire shape

Folder JSON (`manifest.json`) and Pro API JSON (`EvidencePackage`) use the same field names and types:

```json
{
  "schema": "hev.evidence.package/v1",
  "id": "ev_01jd2x8c7k4m5n6p7q8r9s0t1v",
  "state": "sealed",
  "title": "Checkout mobile viewport overflow validation",
  "summary": "Verified 390px viewport layout and payment submit flow on preview build.",
  "source": "browser-use",
  "order": "checkout-mobile-fix-20261120",
  "session": "019a7f3c-1b2e-7000-8a11-4d5e6f7a8b9c",
  "gate": "browser-validation",
  "repo": "hev/app",
  "branch": "fix/checkout-mobile",
  "commit": "9a1c6e4be78c3bd8f0123456789abcdef0123456",
  "host": "mini",
  "author": "d-checkout-mobile-val",
  "role": "agent",
  "tags": ["validation", "checkout", "mobile"],
  "claims": [
    {
      "id": "clm_01jd2x8d01a2b3c4d5e6f7g8h9",
      "criterion": "Submit button stays within 390px viewport with no horizontal scroll",
      "text": "Verified scrollWidth === innerWidth (390px) and button click completes checkout.",
      "verdict": "pass",
      "item_ids": [
        "evi_01jd2x8c90a2b3c4d5e6f7g8h9",
        "evi_01jd2x8ca1a2b3c4d5e6f7g8h9"
      ]
    }
  ],
  "items": [
    {
      "id": "evi_01jd2x8c90a2b3c4d5e6f7g8h9",
      "seq": 1,
      "modality": "image",
      "name": "001-checkout-390px.png",
      "caption": "Checkout form at 390x844 before submit",
      "mime": "image/png",
      "sha256": "3f2a9c1b04de...",
      "size": 148290,
      "width": 390,
      "height": 844,
      "url": "http://localhost:3000/checkout",
      "step": "inspect-layout",
      "captured_at": "2026-11-20T18:10:04Z"
    },
    {
      "id": "evi_01jd2x8ca1a2b3c4d5e6f7g8h9",
      "seq": 2,
      "modality": "video",
      "name": "002-checkout-submit.mp4",
      "caption": "Full click-through from cart to confirmation screen",
      "mime": "video/mp4",
      "sha256": "8b1e4d90c7fa...",
      "size": 4821904,
      "width": 390,
      "height": 844,
      "duration_ms": 14200,
      "poster_sha256": "1c0a7e44b92d...",
      "url": "http://localhost:3000/checkout",
      "step": "submit-flow",
      "captured_at": "2026-11-20T18:10:21Z"
    },
    {
      "id": "evi_01jd2x8cb2a2b3c4d5e6f7g8h9",
      "seq": 3,
      "modality": "text",
      "name": "003-console.log",
      "caption": "Browser console and network status summary",
      "mime": "text/plain",
      "sha256": "c94e12aa01bc...",
      "size": 612,
      "line_count": 14,
      "redacted": true,
      "text": "POST /api/checkout 200 OK (184ms)\n0 console errors\n",
      "captured_at": "2026-11-20T18:10:22Z"
    }
  ],
  "created_at": "2026-11-20T18:10:01Z",
  "updated_at": "2026-11-20T18:10:25Z",
  "sealed_at": "2026-11-20T18:10:25Z"
}
```

### Accepted modalities, byte sniffing, and caps

Every file's type is decided by **sniffing the bytes**, never by filename or `Content-Type`:

| Modality | Accepted MIME types | Rejected examples | Default cap |
|----------|---------------------|-------------------|-------------|
| `text` | Valid UTF-8 `text/plain`, `text/markdown`, `application/json`, `text/x-diff`, `text/csv`, `text/html` (stored as inert text; served sandboxed) | Binary / invalid UTF-8 | 5 MiB per item (inlined in JSON when ≤ 64 KiB, file/blob always written) |
| `image` | `image/png`, `image/jpeg`, `image/gif`, `image/webp` | SVG (can carry scripts), ICO, BMP, TIFF | 10 MiB per item |
| `video` | `video/mp4` (ISO BMFF `ftyp`), `video/webm` (EBML/WebM), `video/quicktime` (MOV `ftyp`) | Unknown containers, truncated/corrupt headers | 100 MiB and `<= 60,000 ms` (60s) per item in v1 |

Package bounds (configurable under `[evidence]` in OSS, returned by `GET /v1/evidence/blobs` in Pro):
- `max_items_per_package`: `64`
- `max_claims_per_package`: `32`
- `max_tags`: `20` (lowercased, deduplicated)
- `metadata` (optional flat string map per item): at most `16` keys, `<= 256` bytes per value

## OSS: folder of files and CLI

### Directory layout

Root directory: `~/.hev/evidence/` (overridable via `[evidence] dir` in `~/.hev/config.toml` or `HEV_EVIDENCE_DIR`).

```text
~/.hev/evidence/
  .lock                          # cross-platform file lock (platformops.LockPath)
  ev_01jd2x8c7k4m5n6p7q8r9s0t1v/
    manifest.json                # EvidencePackage JSON (mode 0600)
    001-checkout-390px.png       # raw item bytes (mode 0600)
    002-checkout-submit.mp4
    003-console.log              # redacted text item bytes (mode 0600)
```

Invariants:
- Directories are created `0700`, files `0600`, written atomically (`platformops.WriteFileAtomic`).
- Each item's stored filename is `<3-digit seq>-<sanitized basename>`, guaranteed to be a single path segment with no `..`, path separators, or control characters.
- Text items and all manifest text fields (`title`, `summary`, `caption`, `claims[].text`, `claims[].criterion`, `metadata`) pass through `internal/redact` before hitting disk.
- Sealing a package (`state: sealed`) sets `sealed_at` and refuses further item or claim appends.
- **No OSS web UI tab:** `hev serve` stays Traces + Board only (memo #551). Operators inspect OSS evidence directly in the folder or via `hev evidence`.

### CLI surface (`hev evidence` + bundled `hev-evidence` skill)

```sh
# One-shot capture (creates and seals in one call)
hev evidence put \
  --title "Checkout mobile viewport overflow validation" \
  --order checkout-mobile-fix-20261120 \
  --source browser-use \
  --tag validation --tag mobile \
  --claim "Submit button stays within 390px viewport" --verdict pass \
  shot.png repro.mp4 console.log

# Incremental capture during a live browser-use or computer-use run
id=$(hev evidence create --title "Admin billing UAT" --order billing-uat --source computer-use)
hev evidence add "$id" before.png --caption "Initial invoice table" --url "http://localhost:3000/billing"
hev evidence add "$id" flow.mp4 --caption "Filter and export flow"
hev evidence add "$id" -m "POST /api/export 200 OK" --name export-status.txt
hev evidence claim "$id" "CSV export downloads without 500" --verdict pass
hev evidence seal "$id"

# List, inspect, export
hev evidence ls [--order NAME] [--session ID] [--modality text|image|video] [--verdict pass|fail] [--tag T] [--since 7d] [--json]
hev evidence show <id> [--json]
hev evidence get <id> [--out DIR]        # verifies sha256 on export; prints folder path in OSS

# Search (lexical grep in OSS; multimodal semantic + hybrid in Pro)
hev evidence search "checkout button overflow" [--also "390px viewport"] [--order NAME] [--modality image|video] [--since 7d] [--json]
hev evidence search --image broken-button.png --modality image --top 10   # Pro only (multimodal query)
```

When `hev evidence search` runs in OSS mode:
- Searches `manifest.json` and text item files with `rg --json` (or Go fallback), matching every word of a phrasing (OR across `--also`), ranking by distinct matched words then newest `created_at`.
- Prints a clear notice that local search matches words in metadata and text items only (images and video are matched via their captions, names, and claims; visual semantic search requires Pro).
- Passing `--image` or `--video` in OSS returns a clear error explaining that visual/video query requires Pro.

## Pro API (`/v1/evidence/*`) and EmbeddingGemma 2 retrieval

Full OpenAPI 3.1 contract: [`openapi/evidence-v1.yaml`](../../openapi/evidence-v1.yaml).

### Pro host disk discipline

When a host is in Pro mode (`internal/storagemode`), `hev evidence` talks directly to `/v1/evidence/*`:
- **No local mirror:** `put`, `create`, `add`, `claim`, and `seal` stream bytes straight to Pro (`/v1/evidence/blobs` or presigned `/v1/evidence/blobs/uploads`) and never write packages or media blobs into `~/.hev/evidence/`.
- **Reads stay remote:** `ls`, `show`, and `search` read from `/v1/evidence/*` (and `/v1/search`). Only an explicit `hev evidence get <id> --out DIR` downloads bytes into the caller-chosen directory (verifying sha256).
- **Clean free-to-paid conversion:** A machine that started in OSS has all local packages neatly isolated in `~/.hev/evidence/`. Logging into Pro switches all new writes and reads to `/v1/evidence/*` with zero local accumulation going forward (an optional `hev evidence push` migration helper can upload prior OSS packages to Pro later if wanted).

### Route summary

| Route | Does |
|-------|------|
| `GET /v1/evidence/blobs` | Return media size/duration caps, per-package limits, and accepted MIME types |
| `POST /v1/evidence/blobs` | Direct content-addressed upload for media up to 10 MiB (returns `201` new / `200` deduped with sniffed metadata) |
| `POST /v1/evidence/blobs/uploads` | Initiate a staged/presigned upload for larger media (e.g. videos up to 100 MiB) |
| `POST /v1/evidence/blobs/uploads/{upload_id}/complete` | Finalize staged upload, sniff/validate container + duration, generate video poster frame, return `EvidenceBlob` |
| `GET /v1/evidence/blobs/{sha256}` | Stream media bytes (`ETag`, `Cache-Control: immutable`, `Accept-Ranges: bytes` for video seeking, `nosniff`, sandbox CSP) |
| `POST /v1/evidence` | Create an `EvidencePackage` (`state: sealed` one-shot or `state: open` incremental) |
| `GET /v1/evidence` | List packages, newest first, cursor-paginated, filterable by provenance, modality, verdict, state, tags, time |
| `GET /v1/evidence/{id}` | Read one `EvidencePackage` with its items, claims, and blob/poster URLs |
| `PATCH /v1/evidence/{id}` | Append items/claims or edit summary while `open`, or transition `state` to `sealed` |
| `DELETE /v1/evidence/{id}` | Delete a package and its search index rows (author or account admin) |
| `GET /v1/evidence/search` | Text-to-multimodal search (`q`, up to 7 `also`, filters, `cursor`, `limit`) |
| `POST /v1/evidence/search` | Multimodal search (optional text `q`/`also` + optional `media[]` image/video query) |

### Layer indexing and `google/embeddinggemma-2`

- **Model:** Layer embeds evidence with **`google/embeddinggemma-2`** (modular 740M; `audio_config=None` loads the 440M text + vision/video stack) into a unified 768d vector space (MRL-truncatable if storage tuning is needed later). Note: `google/embeddinggemma-300m` is text-only and is not used for multimodal evidence.
- **What gets indexed per package:**
  - **Package-level text row:** `title`, `summary`, `claims` (`criterion` + `text`), `tags`, and provenance facets (`order`, `session`, `repo`, `gate`, `source`, `verdict`, `created_at`).
  - **Per-item rows (linked by `package_id`):**
    - `text` item: redacted text content + `caption` + `name` (embedded with EmbeddingGemma 2 text tower + BM25 full-text index).
    - `image` item: raw image pixels (embedded with EmbeddingGemma 2 vision encoder into the same 768d space) + BM25 over `caption`/`name`/`url`/`step`.
    - `video` item: whole clip (`<= 60s`, `<= 100 MiB` in v1) embedded via EmbeddingGemma 2's 8K multimodal video context + BM25 over `caption`/`name`/`url`/`step`.
- **Ranking and package grouping:**
  - For text queries (`GET /v1/evidence/search`), Layer runs hybrid ANN (against the unified text/image/video vector index) + BM25 per phrasing, fused via RRF.
  - For multimodal queries (`POST /v1/evidence/search`), query images/videos are embedded with the same model and scored via ANN alongside any text legs, fused via RRF.
  - Hits are **grouped by `package_id`** (at most one `EvidenceHit` per package, ordered by best score), carrying the `package` summary and `matches[]` (top matching items with `item_id`, `seq`, `modality`, `score`, optional `snippet`, optional `blob_url` / `poster_url`, and reserved optional `segment: {start_ms, end_ms}`).
- **Forward-compatible video segments:** `segment` is omitted (`null`) in v1 whole-clip mode. When Layer later adds sliding-window or scene-cut indexing for longer clips, populated `{start_ms, end_ms}` values light up jump-to-timestamp playback in the Pro UI with no API breaking change.

## Federated search (`/v1/search`)

Pro adds a single cross-surface retrieval endpoint across kit's searchable memory and record surfaces: **`traces`**, **`board`**, **`orders`**, and **`evidence`**.

| Route | Purpose |
|-------|---------|
| `GET /v1/search` | Text search across `in=traces,board,orders,evidence` (default: all four), `q` + up to 7 `also`, shared filters (`since`, `until`, `order`, `session`, `repo`, `host`, `author`, `tag`), `cursor`, `limit` |
| `POST /v1/search` | Same as `GET /v1/search`, plus optional `media[]` (image/video query) and per-surface filter overrides (`filters.traces`, `filters.board`, `filters.orders`, `filters.evidence`) |

### Execution and fusion on Layer

1. Each requested surface is queried in parallel in Layer using its own native hybrid path (`traces`, `board`, `orders`, `evidence`).
2. When a request supplies `media[]` with no text `q`, only `evidence` executes the multimodal ANN query; text-only surfaces (`traces`, `board`, `orders`) are reported under `surfaces` with `status: "skipped"` and `reason: "modality_unsupported"` rather than failing the request.
3. When a request supplies both text `q` and `media[]`, all requested surfaces run text legs and `evidence` additionally runs the multimodal leg.
4. Per-surface ranked lists are fused with Reciprocal Rank Fusion (RRF) into a single `data[]` page of `FederatedHit` objects, each discriminated by `surface` (`traces | board | orders | evidence`) and carrying both normalized preview fields (`id`, `title`, `snippet`, `score`, `timestamp`, `order`, `session`) and the full surface hit (`trace`, `board`, `order_hit`, or `evidence`).

### CLI companion (`hev search`)

```sh
hev search "checkout button overflow"                          # Pro: /v1/search across traces,board,orders,evidence
hev search "checkout button overflow" --in board,orders,evidence
hev search --image broken-button.png --in evidence             # Pro multimodal
```

In OSS mode, `hev search` runs local lexical search over the available folders (`~/.hev/{traces,board,orders,evidence}`), merges by lexical rank + recency, and labels the output as non-semantic word match.

## Hooks

Additive events on kit's hook layer (`internal/hooks` and Pro webhooks):

| Event | Raised when | Payload |
|-------|-------------|---------|
| `evidence-created` | A package is created (`open` or one-shot `sealed`) | `hev.hook.event/v1` with `harness: kit`, package metadata, item counts, redacted claims (never binary media) |
| `evidence-updated` | Items or claims are added to an `open` package | Same, with added item/claim ids |
| `evidence-sealed` | A package transitions `open` → `sealed` | Same, with final item counts and claim verdicts |
| `evidence-deleted` | A package is deleted (Pro) | Package id and actor |

Adds family pattern `evidence-*` to `HookEventName`.

## First-class Evidence viewer (Pro UI)

OSS `hev serve` stays **Traces + Board only** (memo #551). The Pro UI adds **Evidence** as a top-level surface beside Traces, Board, and Orders/Asks:

1. **Evidence explorer:**
   - Unified search bar supporting text queries and drag-and-drop image/video query (calling `POST /v1/evidence/search`).
   - Facet filters: `order`, `session`, `source`, `modality`, `verdict`, `tag`, `since`/`until`, `author`.
   - Card/gallery grid showing package title, thumbnail strip (image previews + video poster frames with duration pill), claim verdict badges (`pass`/`fail`/`inconclusive`), item modality counts, and provenance chips.
2. **Package viewer:**
   - Provenance header with direct links to the **Order**, **Trace session**, **Gate**, and Git commit.
   - Claims panel where clicking a claim highlights its linked `item_ids`.
   - Multimodal viewer:
     - **Images:** high-res lightbox with zoom/pan, dimensions, caption, URL/step metadata, sha256 copy, download.
     - **Video:** native HTML5 player backed by HTTP Range requests (`/v1/evidence/blobs/{sha256}`), poster frame, playback speed controls, and jump-to-timestamp markers when `segment` is present.
     - **Text:** monospace / JSON / diff / log viewer with search-in-file, copy, and download.
3. **Cross-surface embeds:**
   - Order detail and Trace session pages in Pro UI display an **Evidence** strip for packages linked to that `order` or `session`.
   - Federated search UI lets users search across Traces, Board, and Evidence from one bar with surface filter chips.

## Security, redaction, and privacy boundaries

- **Explicit capture only:** Nothing captures screenshots, recordings, or files implicitly. Validators, browser-use/computer-use skills, and operators invoke `hev evidence` or `/v1/evidence` explicitly.
- **Redaction before persistence:** Every text item and manifest text field passes through `internal/redact` before writing to disk or sending to Pro. Media pixels and video frames cannot be text-scrubbed; docs and skill prompts state this clearly.
- **Client data discipline (memo #445):** Evidence is for work-product verification artifacts, not copying prohibited client-system records to disk.
- **Safe media serving:** Sniffed MIME types only; SVG and executable containers refused; media endpoints always emit `X-Content-Type-Options: nosniff` and `Content-Security-Policy: default-src 'none'; sandbox`.
- **Path and resource bounds:** Strict filename sanitization (no path traversal), bounded uploads (`413` over cap without buffering unbounded streams), and timeouts on every network/subprocess call.

## Rollout slices

1. **Phase 0 — Contract & Docs (this RFC):** Review RFC 0022, merge `openapi/evidence-v1.yaml` into `openapi/kit-v1.yaml` (once PR #298 lands), add Evidence OSS/Pro pages and `/v1/evidence` + `/v1/search` reference to the 0.7 docs site.
2. **Phase 1 — OSS Package & CLI (`hev/kit`):** Extend byte-sniffing/media inspection for video + text, build `internal/evidence` (atomic folder store under `~/.hev/evidence/`, redaction, lexical grep), wire `hev evidence` CLI + `hev-evidence` skill + local `evidence-*` hooks.
3. **Phase 2 — Layer Multimodal Embedder & Pro API (`layer-pro` + `kit-cloud`):** Deploy `google/embeddinggemma-2` embedder service (`audio_config=None` default), object-store blob upload with presigned video flow + poster extraction, `/v1/evidence/*` routes, `/v1/search` federated RRF endpoint, and switch `hev evidence` / `hev search` to Pro via `internal/storagemode`.
4. **Phase 3 — Pro UI & Validator Integration:** Ship the first-class Evidence viewer and federated search bar in Pro UI, add inline Evidence strips to Orders and Traces, and wire browser-use, computer-use, and RFC 0019 validator gates to emit `hev evidence` packages automatically.

## Open questions

1. **Video poster frame extraction in OSS vs Pro:** In Pro, the upload completion step extracts a WebP/JPEG poster frame (`poster_sha256`) for the gallery UI. Should OSS stay zero-dependency (no `ffmpeg` required; `poster_sha256` optional/omitted locally) and only require poster extraction server-side in Pro? *(Recommendation: yes — keep the OSS Go binary dependency-free.)*
2. **Retention / TTL for large video blobs in Pro:** Should `EvidencePackage` support an optional `expires_at` / retention tier (e.g. keep embeddings + metadata + poster frame permanently while expiring raw >10 MiB video blobs after N days), or keep raw videos indefinitely under Pro storage billing?
3. **Integrating with RFC 0019 Gates:** Should a Gate verdict record (`internal/gates`) reference `evidence_ids: ["ev_..."]` directly so every `validator` gate pass/fail links straight to its multimodal package? *(Recommendation: yes.)*
4. **Design specs / RFCs in Orders:** With `orders` included in federated `/v1/search`, design specs (`docs/rfcs`) can live as Orders (`state: draft` until dispatched) and be discovered progressively through Layer rather than bloating code repos. The exact `docs/rfcs` archival/retirement cutover is tracked separately.
