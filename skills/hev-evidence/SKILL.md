---
name: hev-evidence
description: Keep proof packages of screenshots, short screen recordings and text from validation, browser-use, computer-use and spike runs. Use when an order or gate asks for evidence, when a check passes or fails and the proof should outlive the run, or when finding what an earlier run proved.
metadata:
  audience: all
---

Package the proof a run produced so a reviewer can see it later: images
(PNG, JPEG, GIF, WebP), videos (MP4, WebM, MOV up to 60s and 100 MiB) and text
(logs, JSON, diffs, CSV, HTML, Markdown up to 5 MiB), with claims about what
they show. Capture is explicit: nothing is recorded unless you add it.

One step, when the run is done:

```sh
hev evidence put --title "Checkout fits a 390px viewport" --source browser-use \
  --order "$HEV_ORDER" --tag validation \
  --claim "Submit stays inside the viewport" --verdict pass \
  shot.png repro.mp4 console.log
```

During a live run, build it up and seal it at the end:

```sh
id=$(hev evidence create --title "Billing UAT" --source computer-use)
hev evidence add "$id" before.png --caption "Invoice table before export" --url http://localhost:3000/billing
hev evidence add "$id" -m "POST /api/export 200 OK" --name export-status.txt
hev evidence claim "$id" "CSV export downloads" --verdict pass --criterion "Export returns 200"
hev evidence seal "$id"
```

A sealed package can no longer change; add a new package instead. Put the
package id in your report.

## Rules

- Say what each image or video shows in `--caption`; captions, names and
  claims are what local search finds in media.
- Verdicts are `pass`, `fail`, `inconclusive` or `info`. Record failures too.
- Text and every caption, title and claim are scrubbed of secrets before they
  are written, but pixels and video frames are not: never capture a screen
  showing a token, a password or client data you may not keep.
- Files are checked by their bytes. SVG, unknown containers, truncated video,
  clips over 60s and files over the caps are refused and nothing is written.

## Find and check

```sh
hev evidence ls --order "$HEV_ORDER" --verdict fail
hev evidence show ID
hev evidence get ID --out ./proof     # verifies every sha256, then copies
hev evidence search "checkout overflow" --also "390px viewport"
```

Search on this machine matches words (every word of a phrasing, `--also` for
other wordings); a miss is not proof nothing was captured. Searching by an
image or a video needs Pro.
