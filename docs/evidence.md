# Evidence

Evidence is kit's sixth data surface, beside traces, the board, orders, peers
and hooks ([RFC 0022](rfcs/0022-evidence.md)). An **evidence package** holds
the proof a run produced: screenshots, short screen recordings and text such as
console logs, JSON receipts and diffs, with optional claims about what they
show. Validators, browser-use and computer-use runs, and spikes are the usual
writers. Capture is explicit: nothing is recorded unless a command adds it.

On a machine with no Pro account, evidence is a folder of self-contained
packages. There is no daemon, no model and no web tab: `hev serve` stays
Traces + Board, and you read packages with `hev evidence` or in the folder.

## Write a package

One step, creating and sealing the package:

```sh
hev evidence put --title "Checkout mobile viewport overflow validation" \
  --order checkout-mobile-fix --source browser-use \
  --tag validation --tag mobile \
  --claim "Submit button stays within 390px viewport" --verdict pass \
  shot.png repro.mp4 console.log
```

`put` prints the package id. `-m TEXT` adds a text item (named by `--name`,
default `message.txt`), and `-` reads one item from stdin. `--claim` may be
repeated; a `--verdict` or `--criterion` pairs with the `--claim` in the same
position, and a single one applies to every claim.

Built up during a live run:

```sh
id=$(hev evidence create --title "Admin billing UAT" --order billing-uat --source computer-use)
hev evidence add "$id" before.png --caption "Initial invoice table" --url http://localhost:3000/billing
hev evidence add "$id" flow.mp4 --caption "Filter and export flow"
hev evidence add "$id" -m "POST /api/export 200 OK" --name export-status.txt
hev evidence claim "$id" "CSV export downloads without 500" --verdict pass --item evi_...
hev evidence seal "$id"
```

A package is `open` until it is sealed. An open package takes items, claims
and edits; a sealed one refuses every change with a conflict error, so add a
new package instead. `put` seals at once.

Flags on `put` and `create`: `--title` (required), `--summary`, `--source`
(`validator`, `browser-use`, `computer-use`, `spike`, `manual`, `api`),
`--order` (default `$HEV_ORDER`), `--session`, `--gate`, `--repo`,
`--branch`, `--commit`, `--host` (default this machine), `--author`, `--role`
(`agent` or `human`) and `--tag`. `add` also takes `--caption`, `--url`,
`--app`, `--step`, `--name` and `--meta KEY=VALUE`. Claims take `--verdict`
(`pass`, `fail`, `inconclusive`, `info`), `--criterion` and, on `claim`,
`--item ID` for supporting items.

## What is accepted

The type of every file is decided by its bytes, never its name or a MIME the
caller gives.

| Modality | Accepted | Cap |
| --- | --- | --- |
| `image` | PNG, JPEG, GIF, WebP, with pixel width and height | 10 MiB |
| `video` | MP4 and MOV (ISO BMFF `ftyp`), WebM (EBML) | 100 MiB and 60 seconds |
| `text` | valid UTF-8, typed as `text/plain`, `text/markdown`, `application/json`, `text/x-diff`, `text/csv` or `text/html` | 5 MiB |

- SVG is refused (it can carry scripts); export it as PNG. So are BMP, TIFF,
  ICO, Matroska, binary files and invalid UTF-8.
- An image must be whole, not just have a readable header: a PNG needs `IHDR`
  first, an `IDAT` and an `IEND`, with every chunk inside the file and its CRC
  matching; a JPEG needs every marker segment inside the file up to the scan
  and an `EOI` after it; a GIF needs at least one image, every block inside
  the file, and its trailer; a WebP's RIFF size must equal the file size, with
  every chunk inside it.
- Video containers are parsed in Go, with no `ffmpeg`: the duration comes from
  `mvhd` (MP4, MOV) or the WebM `Info` timecode scale and duration. Every
  MP4/MOV box (top level and inside `moov`, `trak`, `mdia`, `minf`, `stbl` and
  the other container boxes) and every WebM element (the Segment, each Cluster
  and the blocks inside it) must fit in the file: one that declares more bytes
  than the file holds is a truncated file and is refused, as is a corrupt
  container or one over a cap. A WebM written live (a browser's MediaRecorder:
  Segment and Cluster of unknown size, no duration) is kept without
  `duration_ms`. Packages on this machine have no video poster frame.
- Text is stored as a file and, at 64 KiB or less, also inlined in the
  manifest with its `line_count`.
- Bounds per package: 64 items, 32 claims, 20 tags (lowercased and
  deduplicated, 64 bytes each), and per item at most 16 metadata keys of 256
  bytes. Titles, summaries, captions and claims have the lengths in
  [the contract](../openapi/evidence-v1.yaml).
- Item names are one plain file name: a name with `/`, `\`, `..` or a control
  character is refused. Other characters outside `A-Z a-z 0-9 . _ -` become
  `-`, and the stored name gets a three-digit sequence prefix.

A refused file writes nothing. `put` checks every file before the package
appears, so one bad file leaves no package behind.

## Redaction

Every text item and every manifest string a person or agent writes (`title`,
`summary`, the provenance fields `order`, `session`, `gate`, `repo`,
`branch`, `commit`, `host` and `author`, item names, `caption`, `url`, `app`,
`step`, `metadata` keys and values, and claim `text` and `criterion`) passes
through kit's secret scrubber before it reaches the disk or a hook event. A
found secret becomes `[REDACTED:rule#fingerprint]`; in an item name the marker
is then made file-name safe (`002-REDACTED-github-pat-4f31…-.txt`). A tag that
holds a secret is refused, since tags are matched exactly. Error messages
never repeat a refused name or key in clear. Text items are
marked `redacted: true`, and the `sha256` in the manifest is of the stored,
redacted bytes. Pixels and video frames cannot be scrubbed: do not capture a
screen that shows a credential or client data you may not keep.

## On disk

The root is `$HEV_EVIDENCE_DIR`, else `[evidence] dir` in
`~/.hev/config.toml`, else `~/.hev/evidence`:

```toml
# ~/.hev/config.toml
[evidence]
dir = "~/.hev/evidence"
```

```text
~/.hev/evidence/
  .lock                          # cross-process lock
  ev_01jd2x8c7k4m5n6p7q8r9s0t1v/
    manifest.json                # EvidencePackage JSON
    001-checkout-390px.png
    002-checkout-submit.mp4
    003-console.log              # redacted text
```

Directories are `0700` and files `0600`, every write is atomic, and one-shot
packages are assembled in a staging folder and renamed into place.
`manifest.json` has the same fields as the Pro API's `EvidencePackage`
([openapi/evidence-v1.yaml](../openapi/evidence-v1.yaml)), with
`schema: "hev.evidence.package/v1"` and ids `ev_` + a lowercase ULID (items
`evi_`, claims `clm_`).

## Read, export and search

```sh
hev evidence ls [--order NAME] [--session ID] [--repo R] [--gate G] [--source S] [--author A]
                [--state open|sealed] [--modality text|image|video] [--verdict pass|fail|...]
                [--tag T] [--since 7d] [--until 2026-11-20T00:00:00Z] [--limit N] [--json]
hev evidence show ID [--json]
hev evidence get ID [--out DIR] [--json]
hev evidence search QUERY [--also PHRASING]... [the ls filters] [--limit N] [--json]
```

`--source`, `--modality` and `--verdict` keep packages matching any value
given; every `--tag` given must be on the package. `--since` and `--until`
take a duration back from now (`7d`, `36h`) or a time.

`get` checks every item against the `sha256` and size in `manifest.json` and
prints the package folder. With `--out DIR` it copies the manifest and items
into `DIR`. A changed, missing or damaged file fails the command (non-zero)
and nothing is copied.

`search` is word search, not semantic search. A package matches when its
title, summary, provenance, tags, a claim, an item's name, caption, URL, step
or metadata, or a text item holds every word of `QUERY`, or of one `--also`
phrasing (up to seven). Hits are one per package, ranked by how many distinct
words matched and then newest first, with up to five matching parts and a
snippet each. Images and video are found through their captions, names and
claims only. Search uses `rg --json` to narrow the files to read when ripgrep
is on PATH and a Go walk otherwise (`HEV_GREP=go` forces it); both give the
same hits. The command prints a notice saying so on stderr, and `--json`
answers in the `EvidenceSearchResponse` shape with `route: "phrase.scan"`,
`engine: "rg"` or `"go"` and `semantic: false`. Searching by an image or a
video (`--image`, `--video`) needs Pro and fails with a message saying so.

`--json` on every command prints the wire shape: `EvidencePackage` for `put`,
`create`, `add`, `claim`, `seal`, `show` and `get`; `{"data": [EvidencePackageSummary], "next_cursor": null}`
for `ls`.

## Hooks

Writing evidence raises three observe-only [hook](hooks.md) events on the
writing host: `evidence-created` (a package is created, open or sealed),
`evidence-updated` (an item, claim or edit lands on an open package) and
`evidence-sealed`. Their payload is `hev.evidence.event/v1`
([schemas/evidence_event.schema.json](../schemas/evidence_event.schema.json)):
the package summary with its redacted claims, each item's id, name, type,
size, hash and dimensions, and the ids an update added. It never carries item
bytes or text. `evidence-*` names the whole family.

## Disk usage

`hev s` (or `hev status`) ends its report with what each surface holds under
`~/.hev`:

```text
Disk usage (~/.hev): 1.2 GiB
  traces    1.1 GiB
  board     4.2 MiB
  orders    0 B
  peers     0 B
  hooks     12.0 MiB
  evidence  88.3 MiB
  other     1.5 MiB
```

`traces` counts `~/.hev/traces` and the file archive `~/.hev/archive`,
`board` counts `~/.hev/board` and `~/.hev/canvas`, `orders` counts
`~/.hev/orders` and `~/.hev/dispatch`, and `other` is the rest of `~/.hev`
(config, secrets cache, logs). An evidence folder configured outside `~/.hev`
is counted as `evidence` and added to the total. Dispatch worktrees under
`~/dispatch` and peers state under `~/.local/state/peers-v2` are outside
`~/.hev` and not counted. The walk does not follow symlinks and stops after a
million entries or three seconds; counts are then marked `≥`.

## Pro

On Pro, evidence is written to and read from `/v1/evidence` and nothing is
kept under `~/.hev/evidence`; Pro adds visual and semantic search and a viewer
in the Pro UI. That is a later phase of RFC 0022: today `hev evidence` always
uses the local folder.
