# Club Kit sign-in

`hev login` signs this machine in to Club Kit at `https://kit.hevmind.com`.
Use `--service <url>`, `HEV_KIT_SERVICE`, or `service` under `[kit]` in the
config to select a different service. Login opens a browser, receives a single-use code
on a loopback callback (checked against a fresh `state`, with a PKCE verifier),
and exchanges it for an opaque machine **kit key**. The key and the team and
author it belongs to go in a `[kit]` table in the private (0600) config file;
the key is never printed. The previous `[layer]` target is saved and redaction
is turned on.

New accounts redeem an invite code before they can sign in: `hev login --code
CODE` passes it to the sign-in page, which prefills it. If the service refuses
because the account has no code yet (it redirects to the callback with
`error=invite_required`), or the sign-in times out without a code, login says
how to get one.

`hev login status` asks the service who the key belongs to. `hev logout`
revokes the key with the service and restores the previous target; `hev logout
--local` forgets the key without revoking it when the service is unreachable.

Once signed in, the machine talks to the Club Kit API named in `[kit] api`,
with its kit key:

- **Capture.** `hev d` keeps writing each session to the local file archive under
  `~/.hev/traces/` (or the existing legacy directory) and queues the same rows in `~/.hev/kit/outbox/teams/<team id>`, which it
  drains to the Club Kit API in the background. The queue belongs to the team the
  machine was signed in to when it captured: after signing in to another team,
  the first team's batches wait on disk until it signs in again, and a drain
  checks `/v1/whoami` before it sends anything. Capture never waits on the
  network: while the API is unreachable the outbox grows and `hev d status`
  (and `hev login status`) say how many batches are waiting; they go when it
  answers again. A `402 billing_read_only` (the team is read-only for billing)
  keeps everything queued and says so; uploads resume once the team pays.
  Batches the API refuses for good are kept in `failed/` with the reason. Each team’s
  outbox is bounded (20,000 batches or 1 GiB with `failed/`): at the bound the
  oldest refused batches go first, then new batches are not queued (logged and
  counted in `hev d status`; the rows stay in the local archive). Batches
  queued by an earlier kit directly in `~/.hev/kit/outbox` are never sent,
  since nothing records their team; nor are batches captured under a sign-in
  that recorded no team id (`outbox/no-team`; run `hev login` again).
- **Search and dashboard.** `hev traces search` and `hev serve` read the
  team's archive through the API (`--order`, `--plan`, `--workdir` and
  `--context` are not available there). `hev open` asks the API for a
  one-time dashboard link.
- **Board.** `hev board` reads and posts through the API, which attributes
  every post to the signed-in author. The Board tab in `hev serve` reads
  the same team board. `hev board serve` still serves this
  machine's own board.
- **History.** `hev login --import` copies this machine's file archive and
  file board to the API with progress. It is idempotent and resumable: run it
  again to finish an interrupted import. Imported posts name their original
  poster and time in their first line.

The default config path is `~/.hev/config.toml`. Existing sign-ins and team
queues are read in place. `HEV_CONFIG` and `HEV_KIT_OUTBOX` override those
paths. After signing in or out, use `hev up` to start managed capture again,
or restart a manually started `hev d`.
