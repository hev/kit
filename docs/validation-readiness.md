# Dashboard validation readiness

Before measuring a deployed Kit candidate, identify the listener's bind address,
running artifact and source configuration. A failed localhost connection does
not establish that a listener bound to another interface is down. A binary on
disk does not prove which bytes a running process loaded.

Run the bounded route probe:

```sh
python3 scripts/preflight-dashboard.py http://HOST:PORT
```

It checks the HTML and invalid-window rejection on the sessions and Stats APIs.
It does not request a valid archive window, follow redirects, export transcripts,
or treat a rendered page as backend readiness. Older servers can ignore `limit`,
so adding `limit=1` to a valid archive query is not a safe bounded probe.

For independent migration validation, keep a private candidate manifest containing:

- Exact Kit, Layer and search commits, installed image digests and executable
  hashes; distinguish observed deployment identity from a candidate match.
- Product origins and existing credential references, never credential values.
  Check actual authenticated metadata and bounded query access, then prove denied
  cross-scope access with an existing restricted identity.
- Owned isolated source/destination namespace families and persistent buffer
  fixtures, ownership, reset commands, retained evidence and safe end state.
  Shared cache health and device configuration do not prove a durable CDC ACK.
- Executable fixture setup and API/browser recipes. Browser setup must prevent
  unintended live archive queries during route-only checks.
- Full reconciliation and golden semantics, controlled fixture size, cold/warm
  measurement procedure and explicit failure cases for the exact candidate.

Record each missing capability with its executor owner and remedy. A readiness
probe passing is preparation evidence, not migration, cutover or feature
acceptance. Do not start a full archive scan to discover whether access works.
