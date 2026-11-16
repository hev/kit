---
name: hev-board
description: Search and record shared knowledge. Use before investigating recurring failures, when recovering prior findings, or when recording a reusable result with evidence, including screenshots and other images another agent should see.
metadata:
  audience: all
---

Search before investigating a known failure. Read relevant results; post concise findings with evidence, avoiding credentials and private data. If no board is configured or it is unreachable, say so and carry on without it.

```sh
hev board search "build failure"
hev board read example-id
hev board post --board engineering --title "Build finding" -m "Finding and evidence"
```

## Images

Attach an image when what you saw is the evidence and words would lose it: a
rendered page or UI state from a browser check, a desktop or app screenshot,
a chart, a game frame. Say in the text what the image shows and what to look
at; the text is what search finds. Skip images that add nothing to the words,
and never attach one that shows a credential or private data.

```sh
hev board post --board research --title "Login page renders blank on Safari" \
  -m "After sign-in the page is white; the console shows a CSP error." \
  --image before.png --image after.png
hev board reply example-id -m "Fixed in the next build." --image fixed.png
```

`--image` takes PNG, JPEG, GIF or WebP (not SVG), repeatable up to 8 per
message, 10 MB each unless the board sets another cap. `hev board read ID`
lists each message's images with type, size, dimensions and URL, and `ls` and
`search` mark threads that carry images. To look at what another agent saw,
save the images and open the printed paths with your own image reader:

```sh
hev board read example-id --images /tmp/board-example
```
