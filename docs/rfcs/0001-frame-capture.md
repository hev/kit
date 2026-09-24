# RFC 0001: Frame capture — session-gated screen observations with on-device embeddings

Tracking issue: TBD

**Status: draft, revision 2 — scope widened from terminal-only to
default-deny dev surfaces after review; no implementation started.
The MVP slice targets macOS only.**

## Summary

Teach `hev d` to capture screen observations ("frames") alongside AI
traces: compositor-filtered captures of dev surfaces — the app under
development, the browser verification loop, the editor — taken only
while a hev-tracked agent session is active, embedded on-device
(CLIP-family image vector + OCR/DOM text), deduplicated by perceptual
hash, and stored as frame envelopes next to the trace envelopes they
align with. The free OSS tier is sovereign by construction: frames
and vectors land in `~/.hev` and a local vector index by default, or
optionally in a private bucket the operator controls — the same
warehouse model traces already use. Embedding always happens
on-device; nothing ever touches infrastructure that isn't the
operator's.

Traces are actions without observations — and the trace already *is*
the terminal observation, so terminal frames mostly re-record what we
have. The marginal information lives in everything the trace cannot
see: did the layout actually render on localhost, which docs page the
human checked between turns, which diff they inspected before
accepting. Frames aligned to trace events turn the archive into
(observation, action, outcome) episodes — the structure computer-use
RL consumes — captured from one consenting operator's real work.

## Who this is for

Privateers: independent operators who want to RL their own computer
use — building a personal corpus of (observation, action, outcome)
episodes from their own machine, owned outright, as frontier
capability accelerates. The builder is the first user. This resolves
what looks like a tension elsewhere in this doc: the capturer, the
consenter, and the beneficiary are the same person, so the privacy
doctrine is not a constraint on the product — it is the product. A
privateer wants *maximal* capture of their own work with *zero*
leakage to anyone else's infrastructure — which is exactly
default-deny scope, on-device embedding, and storage that is either
the laptop or a bucket they hold the keys to. Fleet/team capture is
explicitly not the wedge; if it ever arrives it is the paid side,
built on the same envelopes.

## Motivation

- **The archive becomes episodes, not logs.** Frames before and after
  a tool call, plus the trace event between them, are (state, action,
  next state) tuples. Reward signals are already in the traces: exit
  codes, test results, diff acceptance, user corrections.
- **The human verification loop is preference-data gold.** What the
  operator looks at *between* agent turns — localhost, the docs page,
  the dashboard — followed by what they type next is a labeled reward
  signal ("looked at the broken layout, issued a correction"). No
  trajectory dataset has this, because annotation farms have no real
  humans verifying real work they care about.
- **Search and replay get eyes.** `hev find "red error dialog"` over
  CLIP vectors; `hev find "ECONNREFUSED"` over OCR/DOM text;
  `hev replay` interleaving trace events with a film strip. Frames are
  the dataset Map has been waiting for.
- **The niche is empty.** OSS screen-memory tools (screenpipe,
  OpenRecall, Pensieve, Windrecorder, rem) are all the continuous-DVR
  model — ambient 24/7 capture with privacy bolted on as "it stays
  local" — and none align frames to agent actions. RL trajectory
  tooling (OpenCUA/AgentNet, PC Tracker, OSGym, ScaleCUA) proves the
  demand side but is lab infrastructure: annotation campaigns and VM
  farms, not a daemon a privateer leaves running. hev already owns the
  trace spine; frames enrich data the daemon already captures.
- **The privacy stance is the product.** Microsoft Recall and Rewind
  got hammered for continuous, ambient, whole-screen recording.
  "Screen memory that isn't spyware" writes its own comparison page
  against screenpipe's 24/7 tagline.

## What the trace misses (the capture inventory)

In order of value-density during a real agent session:

1. **The app under development.** localhost, the Electron window, the
   simulator. The outcome observation: the trace has the diff and the
   exit code; the screen has whether it actually rendered. Highest RL
   value, lowest creep — content served from localhost is by
   definition the work product.
2. **The browser verification loop.** Docs, Stack Overflow, GitHub
   issues, dashboards — the operator's attention trajectory between
   agent turns.
3. **Editor/IDE state.** The diff actually reviewed before acceptance,
   the file scrolled to. Traces record edits, not what the human
   inspected.
4. **Reference surfaces.** Figma, DB GUIs, API clients, spec PDFs.
5. **The terminal** — mostly redundant with traces. Kept as cheap
   alignment anchors and for the one thing traces genuinely miss
   there: TUI rendering.

Special case: **agent-driven browsing.** When the agent itself drives
a browser (Playwright/CDP, browser skills), do not screenshot pixels —
attach via CDP and capture URL, DOM, and screenshot at full fidelity.
Better-than-pixels observations, trivially consented since the agent
is doing the browsing.

## Decision: session-gated, default-deny content classes, embedding-first

The non-creepy properties are structural invariants, not settings
defaults. What makes Recall/Rewind creepy is not how many windows they
capture — it is **illegibility**: the user cannot say crisply what is
recorded and what never is. So the scope pillar is legible
default-deny rules, not window count:

1. **Session-gated, not always-on.** Frames are captured only while a
   hev-tracked agent session is actively writing trace files. The
   daemon already knows this (scanner over Claude raw-body and Codex
   rollout dirs; response-capture hooks for live events). No agent
   session, no capture — a hard invariant, period.
2. **Default-deny content classes, enforced at the compositor.**
   ScreenCaptureKit's `SCContentFilter` captures a display while
   *excluding* apps from the composite — non-dev apps are not blurred
   or cropped, they are never rendered into the framebuffer hev sees.
   The allowlist is dev surfaces: browsers, editors, terminals,
   simulators, design tools. Slack, Mail, Messages never exist in the
   capture. Anything unlisted is denied.
3. **URL-gated browser capture.** Pixels cannot tell localhost from a
   bank tab, so browser capture is gated by a lightweight extension or
   CDP attach: localhost/127.0.0.1 always; a domain allowlist for
   docs/GitHub/dashboards; private/incognito windows never; unmatched
   URLs dropped by default. The extension also yields DOM text instead
   of OCR — higher fidelity, and redaction runs on structured text.
4. **Hard denies, non-configurable.** Password managers, secure-input
   fields, private browsing windows — denied regardless of any list.
5. **Visible indicator.** Menu bar dot / TUI badge whenever capture is
   live; the macOS Screen Recording TCC prompt and OS-drawn indicator
   vouch that nothing is hidden.
6. **Sovereign-first, embedding-first.** Embedding always runs
   on-device. Storage is on-device by default, or the operator's own
   private bucket — the same warehouse traces already ship to, keys
   held by the operator, no hosted service. Frame upload is
   per-profile opt-in and can be stricter than the trace policy
   (e.g. "embeddings sync, pixels don't").
7. **Embed-then-discard mode.** The strictest tier: raw pixels live in
   memory only long enough to compute the embedding, pHash, and text,
   then drop. Only possible because embedding is on-device.
8. **Retention by default.** Raw frames TTL (default 7 days), vectors
   and text kept. `hev d pause` as kill switch; everything
   inspectable via `hev ls --frames`.

The legibility test: `hev frames policy` prints the effective rules in
one screen, every rule default-deny. *"hev can't see anything that
isn't a dev surface — here's the filter, it runs in the OS compositor,
go read it"* is a stronger claim than "terminal only," and it scales
to the surfaces where the value actually is.

A second decision, on triggers: **capture on trace events, not a
clock.** Frames fire on turn start, tool result, and turn end — hooks
deliver these with low latency where installed; the scanner's
file-event detection is the fallback gate. Event-triggered capture
aligns frames to actions (the RL requirement), and combined with pHash
dedup it means almost nothing is stored unless something visibly
changed. Expect 10–100x volume collapse versus any DVR model.

## Capture-scope tiers

Orthogonal to the storage enum (`pixels | embeddings | off`):

| Tier | Scope | Mechanism |
|------|-------|-----------|
| 0 | Terminal only | SCK window capture; timid default, alignment anchors |
| 1 | Dev surfaces | `SCContentFilter` app allowlist — the intended default |
| 2 | + browser, URL-gated | Extension/CDP; localhost + domain allowlist, DOM text |
| 3 | Full display minus denylist | If it exists at all, embed-then-discard only |

## Goals

- A `frames` capability in the daemon: capture backend, embedder, and
  frame-envelope writer behind interfaces, macOS implementations
  first.
- A signed Swift sidecar (`hev-frame`) owning ScreenCaptureKit capture
  with `SCContentFilter` allowlisting, MobileCLIP embedding (Core ML /
  ANE), and Vision-framework OCR, speaking a stdio protocol to the Go
  daemon.
- Frame envelopes carrying session/turn ids, trigger, timestamp,
  source metadata (app, URL where known), pHash, embedding, redacted
  text (OCR or DOM), and an optional content-addressed WebP blob —
  same envelope discipline, ledger dedup, and capture-policy salt as
  trace envelopes.
- Frame text routed through the existing redaction path (`redact.go`),
  so secret patterns apply to what was on screen for free.
- A local vector index (sqlite-vec, single file in `~/.hev` — no
  hosted service) and `hev find <text>`: CLIP text encoder → nearest
  frames → jump into the TUI at that point in the session.
- `hev frames policy` — print the effective capture rules; the
  legibility test as a command.
- Frame strip in the TUI trace view.
- Storage policy and scope tier shaped as enums from day one, so
  embed-then-discard and tier changes are flags, not refactors.

## Non-goals

- **Continuous/ambient recording.** Never, in any mode. Not a config
  option.
- **Keylogging or input capture.** Trace events are the action stream;
  raw keystrokes add creep without adding signal hev needs.
- **Communication surfaces.** Slack, mail, messages stay outside every
  tier; requirements arriving there reach the corpus through the
  trace (the prompt), not the screen.
- **The browser extension and CDP attach** — tier 2 is post-MVP; the
  envelope's source metadata anticipates it.
- **Bucket sync for frames in the MVP** — deferred one slice, not
  philosophically: it rides the existing uploader/policy path and the
  `upload` enum below, so it is plumbing, not design. Likewise
  Linux/Windows backends and embed-then-discard enforcement; schema
  and enums anticipate all three. Linux arrives via
  xdg-desktop-portal, consent-based by design.
- **Audio.** Different consent surface entirely.
- **Fleet/team capture, training pipelines, hosted retrieval.** The
  open-core line: OSS = capture + on-device embed + search + replay
  for one operator, stored locally or in their own bucket; those are
  the paid side.

## Vocabulary

- **frame** — one screen-observation event, with or without retained
  pixels.
- **frame envelope** — the stable wrapper: session/turn ids, trigger,
  timestamp, source metadata, pHash, embedding, text, optional blob
  ref.
- **dev surface** — an app class on the capture allowlist: browsers,
  editors, terminals, simulators, design tools.
- **session gate** — the invariant that capture runs only while a
  tracked agent session is active.
- **content-class filter** — the compositor-level allowlist
  (`SCContentFilter`): unlisted apps are never rendered into the
  capture.
- **URL gate** — the browser-side rule set: localhost always, allowed
  domains, private windows never, unmatched denied.
- **pHash dedup** — perceptual-hash threshold skipping frames visually
  identical to the previous one.
- **embed-then-discard** — the storage policy where pixels never
  persist; only embedding + pHash + text do.
- **episode** — an exported (frame, trace event, result, next frame)
  tuple sequence with reward signals from the trace.
- **privateer** — an independent operator capturing their own
  computer use as a personal corpus; the target user, and the author.

## Proposed surface (sketch)

Config (`~/.hev/config.toml`):

```toml
[frames]
enabled = true
scope = "dev-surfaces"     # terminal | dev-surfaces | browser | display
storage = "pixels"          # pixels | embeddings | off
retention_days = 7          # raw blobs only; vectors and text kept
apps_allow = []             # additions to the built-in dev-surface list
capture_window_titles = false   # titles leak secrets; off by default
upload = "off"              # off | embeddings | all — ships to the
                            # active private bucket, same as traces

[frames.browser]            # tier 2, post-MVP
domains_allow = ["localhost", "127.0.0.1", "github.com"]
```

CLI:

```
hev find "red error dialog"      # CLIP text-encoder search over frames
hev frames policy                # print effective capture rules
hev ls --frames                  # sessions with frame counts
hev trace <id> --frames          # frame strip interleaved with events
hev d pause                      # kill switch, frames and traces
```

Frame envelope (abridged):

```json
{
  "kind": "frame",
  "session_id": "…",
  "turn": 14,
  "trigger": "tool_result",
  "ts": "2026-06-11T17:03:21.412Z",
  "source": {"app": "com.google.Chrome", "url": "http://localhost:3000/", "title": null},
  "phash": "c3a1…",
  "embedding": {"model": "mobileclip-s2", "dim": 512, "v": [/* … */]},
  "text": {"kind": "ocr", "value": "…redacted via capture policy…"},
  "blob": {"sha256": "ab12…", "format": "webp"}
}
```

The blob is content-addressed, which drops straight into the
checkout/replay Merkle direction: frames are leaves in the same
layout, and `hev replay <id>` becomes trace events interleaved with a
film strip.

MVP slice, in order:

1. `hev-frame` sidecar: SCK capture with `SCContentFilter` allowlist +
   MobileCLIP embed + Vision OCR, stdio protocol.
2. Daemon: session-gated triggers (hooks where installed, scanner
   file-events otherwise), pHash dedup, frame envelopes to local store
   only. Scope tiers 0–1.
3. sqlite-vec index + `hev find`, `hev frames policy`.
4. TUI frame strip.

## The CLIP caveat

CLIP is trained on natural images and is mediocre on dense text
screens — two terminals full of different stack traces can embed
nearly identically. Widening scope to rendered UIs (localhost, docs,
design tools) plays *toward* CLIP's strengths, but the design still
never bets on CLIP alone: every frame carries text (Vision OCR, or DOM
text at tier 2) alongside the vector. CLIP buys visual clustering and
"find screens that look like this"; text buys "find the session where
`ECONNREFUSED` was on screen." For RL data the text plus structure may
prove *more* valuable than the vector; capture both and let the data
decide. The embedder sits behind an interface so SigLIP or a
screenshot-tuned encoder slots in without schema changes.

**Pressure-test before building:** embed ~50 real frames (terminals
*and* rendered localhost UIs) with MobileCLIP and inspect
nearest-neighbor sanity. If CLIP-on-screens is weak, the MVP leans
text-first with CLIP as the clustering layer — same architecture,
different headline.

## Dependencies and release placement

- Extends the existing daemon spine: scanner (session gate), envelope
  (new `frame` kind), policy (capture-policy salt grows frame fields),
  redact (frame text path), ledger (dedup), store (blobs + index).
- Composes with the checkout/replay direction (content-addressed
  blobs, Merkle layout) and feeds Map (first embedding corpus).
- `hev export --episodes` should target an existing trajectory format
  (OpenCUA/AgentNet-shaped) so privateers' corpora are usable by the
  training ecosystems that already exist. Export is post-MVP but the
  envelope fields it needs (trigger, turn alignment, source) are MVP.

## Open questions

- **The default dev-surface list.** Which apps ship on the built-in
  allowlist, and how it is versioned — the list *is* the privacy
  promise at tier 1, so changes to it deserve the same visibility as
  the capture-policy salt.
- **Capture geometry at tier 1.** Whether the filtered display
  composite is stored as one frame or split per-window per-app —
  per-window embeds and retrieves better; composite preserves spatial
  attention layout. Possibly both: composite blob, per-window
  embeddings.
- **Trigger latency without hooks.** Scanner file-events lag the
  action by up to the scan interval; whether that misalignment is
  acceptable for episodes or hooks become a soft requirement for the
  RL story.
- **Human-loop sampling between turns.** Trace events stop while the
  operator verifies; capturing the verification loop may need a
  low-rate sample (or input-driven trigger — focus change) *within*
  an active session window. Tension with "no clock" is real and needs
  a crisp rule, e.g. "focus-change events only, still session-gated."
- **Browser mechanism.** Extension vs CDP-attached dev profile vs
  both; extensions are a second consent surface and a maintenance
  burden across browsers.
- **Sidecar distribution.** Signing and notarizing `hev-frame`, and
  whether model weights ship in the binary or download on first use.
- **CLIP quality on screens** — the pressure-test above; decides the
  headline, not the architecture.
- **Window titles.** Policy-gated off by default; whether any tier
  should capture them at all, given how often titles embed paths and
  secrets.
- **Multi-display and virtual desktops.** Whether capture follows the
  session's surfaces across displays or pins to one.
- **Episode export format.** Which trajectory schema to target first
  (OpenCUA/AgentNet vs something RLDS-shaped) and what the reward
  field vocabulary is.
- **Numbering and naming.** Whether frames stay a daemon capability
  (`[frames]` in config) or become a named kit piece alongside
  Capture/Loop/Map/Layer/Mesh.

## References

- Prior art, screen-memory camp: [screenpipe](https://github.com/screenpipe/screenpipe)
  (YC S26; MIT source, paid license; 24/7 screen+audio, OCR-first),
  [OpenRecall](https://github.com/openrecall/openrecall) (AGPL,
  screenshots + OCR + text embeddings),
  [Pensieve](https://github.com/arkohut/pensieve), Windrecorder, rem.
  All continuous-DVR; none trace-aligned.
- Prior art, RL-data camp: [OpenCUA / AgentNet](https://arxiv.org/pdf/2508.09123)
  (annotation tool: screen video + input + accessibility trees),
  [PC Tracker](https://arxiv.org/html/2505.13909v1),
  [OSGym](https://arxiv.org/pdf/2511.11672) (1024 OS replicas,
  synthetic rollouts), [ScaleCUA](https://arxiv.org/pdf/2509.15221).
  All lab infrastructure; none capture one operator's real consented
  sessions.
- [ScreenCaptureKit / SCContentFilter](https://developer.apple.com/documentation/screencapturekit/sccontentfilter)
  — compositor-level app include/exclude; the tier-1 primitive.
- [MobileCLIP](https://github.com/apple/ml-mobileclip) — Apple's
  on-device CLIP family, Core ML / ANE path.
- [sqlite-vec](https://github.com/asg017/sqlite-vec) — single-file
  local vector index; fits "no hosted service required."
- Internal: checkout/replay Merkle direction (memory:
  `project_checkout_replay_direction`); wire-capture stance — the
  daemon captures missing sources, no MITM (memory:
  `project_wire_capture_stance`).
