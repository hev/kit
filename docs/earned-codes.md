# Earned Field Kit codes

The CLI defaults to `https://kit.hevmind.com`. Resolution order is `--service`
(login only), `HEV_KIT_SERVICE`, `[kit] service`, then the hosted default.

A successful sender-side dispatch start increments a local counter. Failed
starts, queued orders (which have not launched), retries and reworks do not
increment it. On the fifth successful launch, an unsigned-in machine starts a
background helper. Dispatch never waits for the network. The helper has a
2-second total request deadline, including response reading, and follows no
redirects. Concurrent helpers share a local lease; an interrupted helper can be
retried after 10 seconds on a later dispatch.

## Service contract

`POST /v1/codes/earned`, `Content-Type: application/json`, no authentication:

```json
{"install_id":"random-persistent-256-bit-base64url-id","dispatches":5}
```

Those are the only fields. No hostname, account, repository or order content is
sent. The random ID is created at the first eligible request and persisted under
`~/.hev/kit/signup.json` (private directory and file). The service must make
issuance idempotent by install ID, since a timeout can occur after minting.

Success: HTTP 200 with:

```json
{"code":"CODE_123","credit_text":"campaign credit","expires":"2026-12-01T00:00:00Z"}
```

`expires` is an RFC3339 UTC timestamp; codes use ASCII letters, numbers, hyphens
and underscores (1–256 characters). Credit text is at most 512 bytes with no
control characters. The response is limited to 16 KiB. The service owns minting,
credit amounts, eligibility, expiry and rate limits; no credit is promised by
the CLI before a valid response.

HTTP 204 or any 4xx ends this installation's campaign attempt without output.
Network errors, 5xx, malformed or expired responses are silent and retry on a
later successful dispatch. A valid code is stored before the one-time award
message. `hev login --help` and `hev status` show the stored reminder until
sign-in or expiry. Code issuance is never performed by help or status.

## Hints and privacy controls

`HEV_NO_HINTS=1` or top-level `hints = false` in `~/.hev/config.toml` disables
hints and earned-code network requests. Successful launches still count locally.
`--json` suppresses hints, awards and network requests for that invocation.
No hint is written to stderr or on a command error. A configured kit key
suppresses all signup messaging (a service-only `[kit]` table is not signed in).

Board, trace search and remote dispatch use at most one hint per family per UTC
calendar day. Human-formatted non-TTY output is explicitly eligible: agents use
these commands and can relay the single line. JSON is always ineligible. State
updates use atomic replacement and a cross-process lock. No network lock is held
by dispatch launches.
