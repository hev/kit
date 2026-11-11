# Subscriptions

`hev subscriptions` shows which flat-rate agent plans this machine is signed
in to, how much of each usage window is used, and when each resets. Check it
before sending heavy work to a machine.

It covers the plan side. The spend side, `subscriptions.toml` priced by
`hev cost-price` and shown by `hev serve`, is unchanged and not required.

## Commands

```
hev subscriptions                 list every subscription (same as ls)
hev subscriptions ls              list every subscription
hev subscriptions show NAME       every meter for one subscription
hev subscription NAME             same as show NAME
hev subscription                  same as ls
```

`subscription` and `subs` are aliases of the group. The names are `claude`
and `codex`. An unknown name exits 1 and lists the valid ones.

Flags, on every form:

| Flag | Meaning |
| --- | --- |
| `--json` | Print the versioned JSON shape below |
| `--refresh` | Ask the provider now instead of serving a cached reading (still one request, no retry) |
| `--host HOST` | Read on a configured dispatch host instead of this machine |
| `--config PATH` | Dispatch configuration that defines `--host` |
| `--plans PATH` | `subscriptions.toml` used to name matching spend plans (default `~/.hev/subscriptions.toml` when present) |

`ls` always exits 0 unless the command itself fails: with no network it shows
the cached reading, marked stale, or `?`/`-` where nothing is known.

### Sample output (synthetic fixtures)

```
$ hev subscriptions
NAME    HARNESS  PLAN  STATE      WEEKLY  RESETS   AGE
claude  claude   max   signed in  44%     in 3d6h  0s
codex   codex    plus  signed in  27%     in 1d6h  1h
```

```
$ hev subscription claude
claude
harness:   claude
plan:      max
state:     signed in
source:    Claude OAuth usage
read:      2030-03-06T12:00:00Z (0s ago)

METER         USED  RESETS                          NOTE
session (5h)  9%    2030-03-06T15:00:00Z (in 3h)
weekly        44%   2030-03-09T18:00:00Z (in 3d6h)
```

`WEEKLY` shows `?` when the provider gave no percentage and `-` when there is
no weekly meter (not signed in, or no reading). `AGE` gains `(stale)` when the
reading is old or a refresh failed; `show` prints the reason. A `spend:` line
in `show` names the `subscriptions.toml` plans configured for the same harness.

States: `signed in`, `not signed in`, `login expired`, `unknown`.

## Where readings come from

### Codex

No network call. Codex records `payload.rate_limits` on every `token_count`
event in its rollouts, `$CODEX_HOME/sessions/YYYY/MM/DD/*.jsonl`
(`CODEX_HOME` defaults to `~/.codex`). A read opens only the six most recently
modified rollouts across the newest seven day directories and keeps the newest
reading per `limit_id`.

- Windows are classified by `window_minutes`, not by slot: a plan may report
  its weekly window as `primary` with `secondary` null. 10080 minutes is the
  weekly meter; a day or less is a session window.
- `plan_type` is the plan tier; `credits` becomes a `credits` meter.
- A `limit_id` other than `codex` becomes a model-scoped meter named by
  `limit_name`.
- A reading taken before its window's `resets_at` has passed is shown as 0%
  used and `rolled over`; the next reset is unknown until a newer reading.
- A reading older than six hours is marked stale.
- State comes from `auth.json`: `chatgpt` is signed in; an API-key login is not
  a plan and shows not signed in. Codex's local files carry no expiry signal,
  so Codex never shows `login expired`.

The parsing is shared with `internal/cost`, which reads the same structure
from recorded accounting rows.

### Claude

The Claude Code OAuth login, read at use: `CLAUDE_CODE_OAUTH_TOKEN`, else the
macOS keychain item `Claude Code-credentials`, else
`$CLAUDE_CONFIG_DIR/.credentials.json` (default `~/.claude`). The credential
JSON carries the plan tier (`subscriptionType`, with the multiplier from
`rateLimitTier`); a token from the environment does not. If the credential's
access token has expired the state is `login expired` and no request is made;
running Claude Code refreshes it.

Usage is one `GET https://api.anthropic.com/api/oauth/usage` with
`anthropic-beta: oauth-2025-04-20`. The endpoint is undocumented and rate
limits hard, so:

- The last reading and the time of the last attempt are cached in
  `~/.hev/subscriptions/claude.json` (mode 0600). Within **10 minutes** of the
  last attempt the cache is served with its age and no request is made, whether
  that attempt succeeded or failed. Ten minutes keeps a watch loop or several
  agents to six requests an hour per machine, while a weekly meter rarely moves
  more than a point or two in that time. `--refresh` forces one request.
- One attempt per read, never a retry. 401 is `login expired`. 429, another
  status, or no network serves the cached reading marked stale with the reason,
  or no meters when there is no cache.
- The cache holds meters, the plan tier and timestamps only. The token is
  never written to it, to a log, an error or a process argument.

The response is read in either of two shapes: a `limits[]` list (`kind` of
`session`, `weekly_all`, `weekly_scoped` with `scope.model.display_name`;
`percent`; `resets_at`) or keyed windows (`five_hour`, `seven_day`,
`seven_day_<model>` with `utilization` and `resets_at`). `extra_usage`
becomes an `extra usage` meter in both.

## Reading another host

`--host HOST` runs the same read on a dispatch host from your dispatch config,
over the `hev dispatch rpc` SSH channel that `hev dispatch` and
`hev admission` use. It reads the host's own logins, rollouts and cache. Plans
are account-wide, so a busy host often has the freshest Codex reading. Only an
owner key can ask; a teammate's restricted key is refused. The host needs a
`hev` that includes `hev subscriptions`.

## JSON

Schema `hev.subscriptions/v1`. Fields are only added within a version; removing
or retyping one bumps it. An unknown value is `null` with a sibling `*_reason`
string, never `0`.

`hev subscriptions --json`:

```json
{
  "schema": "hev.subscriptions/v1",
  "generated_at": "2030-03-06T12:00:00Z",
  "subscriptions": [ STATUS, ... ]
}
```

`hev subscriptions show NAME --json`:

```json
{
  "schema": "hev.subscriptions/v1",
  "generated_at": "2030-03-06T12:00:00Z",
  "subscription": STATUS
}
```

STATUS:

| Field | Type | Meaning |
| --- | --- | --- |
| `name` | string | `claude`, `codex` |
| `harness` | string | harness the plan serves |
| `host` | string, optional | set when read with `--host` |
| `plan` | string or null | plan tier; `plan_reason` when null |
| `state` | string | `signed_in`, `not_signed_in`, `login_expired`, `unknown`; `state_reason` explains |
| `meters` | array of METER | every meter, empty when there is no reading |
| `weekly` | METER or null | the plan-wide weekly meter; `weekly_reason` when null |
| `source` | string | where the reading came from; `(cached)` when served from cache |
| `observed_at` | RFC 3339 or null | when the reading was taken; `observed_reason` when null |
| `age_seconds` | integer or null | age of the reading at `generated_at` |
| `stale` | bool | the reading is old or a refresh failed; `stale_reason` says which |
| `configured_plans` | array of string | `subscriptions.toml` plans on the same harness |

METER:

| Field | Type | Meaning |
| --- | --- | --- |
| `kind` | string | `session`, `weekly`, `weekly_scoped`, `window`, `credits`, `extra_usage` |
| `label` | string | display name, such as `weekly (Opus)` |
| `scope` | string, optional | model for `weekly_scoped` |
| `window_minutes` | integer, optional | window length |
| `percent` | number or null | percent used, 0 to 100; `percent_reason` when null |
| `resets_at` | RFC 3339 or null | next reset; `resets_at_reason` when null |
| `rolled_over` | bool, optional | the reading predates a reset that has passed; `percent` is 0 |
| `detail` | string, optional | credits balance, or extra usage `on`/`off` |

## Adding a subscription

Add a `Provider` to `Providers()` in `internal/subscriptions` with a `Read`
function that returns a `Status`. Normalisation (weekly headline, reasons for
unknowns, roll-over, configured plans) is shared. Harnesses with no plan to
check, such as pi and opencode, have no row.
