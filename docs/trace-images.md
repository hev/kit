# Images in traces

An agent that takes screenshots, a person who pastes a mock, a computer-use
rig that learns from its own frames: the pictures are part of what happened.
Trace capture keeps them, so the next agent can look at what the last one saw.

## What is captured

`hevd` and `hev index` read every image a transcript carries inline, in a
user message or in a tool result:

| Harness | Where images appear | What the transcript holds | Captured |
| --- | --- | --- | --- |
| Claude Code | user messages; tool results (screenshots, `Read` of an image) | `{"type":"image","source":{"type":"base64","media_type":…,"data":…}}` | yes |
| Codex | user messages (`input_image`); tool output arrays (`view_image`, browser and computer-use screenshots) | `image_url` as a `data:image/…;base64,…` URL | yes |
| Pi | user, tool-result and custom messages | `{"type":"image","data":<base64>,"mimeType":…}` | yes |
| OpenCode 1.x | file parts on prompts; `attachments` on completed tools | `url` as a `data:` URL for images | yes |
| OpenCode 2.x | prompt `files` and tool `content` | inline `data` or a `data:` URI; or a `media:<sha256>` reference into OpenCode's own `blob` table | inline yes; references are recorded as not inline |
| dot | none | dot drops photos before writing a trace and records only a count in the text | nothing to capture |

A `url` or `file` image source (a reference, not bytes) is recorded with a
marker that says so; kit does not fetch it.

## Where the bytes live

On the capturing host, under the archive directory, in **both** storage
modes:

```text
~/.hev/archive/<namespace>/blobs/sha256/<first 2 hex>/<sha256>
```

(`[archive] dir` or `HEV_ARCHIVE_DIR` moves the root, as for the rest of the
archive.) This is the content-addressed store in `internal/blob`, shared with
the board: atomic writes, one file per distinct image however many sessions
show it, the media type sniffed from the bytes (PNG, JPEG, GIF and WebP;
everything else, SVG included, is refused) and a 10 MB cap per image. The
directory is created on the first image, so a host that never sees one has
none.

Layer has no object store yet, so in Layer mode the rows carry only the
metadata and the bytes stay on the host that captured them. Nothing about
images is ever sent to Layer except that metadata.

## What the rows carry

Each block that had images gets an `images` list, in order:

```json
[{"index":1,"sha256":"3f2a…","type":"image/png","size":7642,"width":390,"height":700}]
```

`index` is the image's position among the block's content parts (in a tool
result whose content is text, image, text, the image is 1; an image that stood
alone in a message becomes its own block, at 0). In the file archive it is a
field of the block row; on Layer it is a string attribute holding that JSON
(declared only on batches that have images). Rows of sessions without images
are byte-for-byte what they were before images were captured
(`TestNoImageRowsMatchBase`).

## Markers in the text

Where an image was, the block's text holds a short stable marker:

```text
[image image/png 390x700 7.5 KB sha256:3f2a9c1b04de]
[image image/png 390x700 7.5 KB sha256:3f2a9c1b04de skipped: image capture is off]
[image image/png 4000x3000 12.3 MB sha256:9b1c… skipped: image is too large: 12.3 MB is over the 10.0 MB limit]
[image skipped: not an accepted image: the bytes are text/plain, and only PNG, JPEG, GIF and WebP are accepted]
```

A reader of the text sees that a picture was there, and a search for `image`
or `image png` finds sessions with pictures. Tool results are not indexed for
search by default, and most screenshots arrive in tool results, so a tool
result with images also gets one small searchable chunk holding just its
markers (block type `image`).

An image that is over the cap or is not an accepted type is not stored; its
marker says it was skipped and why. A pasted image is never taken as the
session's first prompt.

## Turning it off

Redaction works on text and cannot see pixels: a screenshot of a terminal can
show a secret that the text scrubber would have removed. Image capture is on
by default because the bytes never leave the host, the same host that already
holds the original transcript. To stop storing image bytes:

```toml
[capture]
images = false
```

or `HEVD_CAPTURE_IMAGES=0` (which wins over the config). With capture off,
markers and metadata are still written and no bytes are stored; the `blobs`
directory is not created.

## Looking at them

```bash
hev traces show <session>              # image markers inline; ▣ lines for tool-result images
hev traces images <session>            # block, role, kind, type, dimensions, size, sha256
hev traces images <session> --out DIR  # save them; prints each path
hev traces images <session> --json     # the same fields, plus on_host and path
```

`--out` writes `NNN-<sha256 prefix>.<ext>` in transcript order and checks
each file's hash on the way out. An image captured on another host is listed
with that host's name and is not saved; `hev traces images` exits non-zero
when any image it was asked to save is not on this host.

`hev serve` shows a session's images inline in the transcript view (each
links to the full image) when the bytes are on this host, and otherwise the
marker with the name of the capturing host. It serves them from
`/api/image/<sha256>` with the sniffed `Content-Type`,
`X-Content-Type-Options: nosniff` and a `default-src 'none'; sandbox` CSP.

## Not done yet

- **Backfill.** Sessions archived before this change have no image metadata.
  Re-indexing them from transcripts still on disk (`hev index --force`) would
  add it; transcripts already gone cannot be recovered.
- **Fetching from another host.** `hev traces images` and `hev serve` only
  read this host's blob store.
- **OpenCode 2.x `media:` references.** The bytes are in OpenCode's SQLite
  `blob` table; kit records the reference but does not read the table.
