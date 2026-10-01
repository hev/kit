# Transcript scrubbing acceptance

The pinned gitleaks v8.24.3 policy has 207 text regex rules. The deterministic
`internal/redact/fixture` generator builds one qualifying specimen per regex,
including its keyword context and entropy threshold, plus nine kit envelope
specimens: PEM private key and certificate, Authorization, URL credentials,
Slack xoxe, sk, tpuf, JWT and high-entropy assignment. The seed is fixed; no
fixture is a valid credential. Path-only rules cannot inspect transcript text
and are excluded. Overlapping detections can win under a different rule name;
counts measure replacements, not every matching detector.

`TestRedactionParsedFixtureCoverage` writes real-format Claude and Codex JSONL
and checks each specimen in a prompt, a command/tool argument and an env/tool
result: 216 × 3 × 2 = 1,296 payload checks. These run in ordinary CI, along with
policy, summary-model, opt-out, counts and stateful migration tests. This corpus
is one specimen per rule, not exhaustive coverage of each regex's alternatives.

## Real stores

`TestLiveRedactionAcceptance` is opt-in. The target config supplies endpoint,
key and store only. An empty key uses normal `daemon.LayerKey` resolution before
HOME is isolated. The test creates a fresh `kit-redact-<timestamp>` namespace,
HOME, config, source roots, salt and migration journals. It never runs launchd
or changes the operator's config, archive or capture daemon. Only synthetic
credentials leave the machine. It deletes its own four namespaces on exit,
including failed assertions; cleanup failures fail the test. It needs permission
to create, write, query and delete namespaces and uses billable hosted embeddings.

```sh
HEV_REDACTION_LIVE_CONFIG=/path/to/target.toml \
  go test ./cmd/hev -run '^TestLiveRedactionAcceptance$' -count=1 -timeout 45m -v
```

The real Postgres lane can use the repository's supported Compose stack. Choose
an unused project and an OS-assigned loopback port; do not restart an existing
stack:

```sh
DO_NOT_TRACK=1 GATEWAY_PORT=0 TURBOPUFFER_API_KEY= \
  docker compose --env-file /dev/null -p kit-redaction-acceptance \
  -f internal/local/docker-compose.yml \
  -f internal/local/docker-compose.postgres.yml \
  up -d --wait --wait-timeout 180 gateway

docker compose --env-file /dev/null -p kit-redaction-acceptance \
  -f internal/local/docker-compose.yml \
  -f internal/local/docker-compose.postgres.yml port gateway 8080
```

Put that port in a separate mode-0600 target config:

```toml
[layer]
endpoint = "http://127.0.0.1:<assigned-port>"
api_key = "local"
store = "pgvector"
```

For hosted Turbopuffer, point the test at its gateway or native endpoint config with
`store = "turbopuffer"` (or omitted, the hosted default). Namespace and source
settings in the target config are ignored. Provider rate limits and other
upstream errors fail the run; they are not counted as passing acceptance.
Reads use four bounded workers by default. Set `HEV_REDACTION_LIVE_READERS=1`
for serial retrieval on a constrained endpoint (valid values are 1–4).
`HEV_REDACTION_LIVE_MODEL` optionally selects a supported model for this new
fixture namespace and both its legacy and current CLI writers; otherwise the
store default is used. It does not alter an existing namespace.

For an actual pre-upgrade baseline, build the last pre-redaction commit in a
scratch directory and pass its binary:

```sh
legacy_dir=$(mktemp -d)
git archive e57a7a8 | tar -x -C "$legacy_dir"
(cd "$legacy_dir" && go build -o "$legacy_dir/hev" ./cmd/hev)
HEV_REDACTION_LIVE_CONFIG=/path/to/target.toml \
HEV_REDACTION_LEGACY_BINARY="$legacy_dir/hev" \
  go test ./cmd/hev -run '^TestLiveRedactionAcceptance$' -count=1 -timeout 45m -v
```

Without that variable, the baseline is seeded with current explicit opt-out.
The test seeds raw summaries as well as chunks, blocks and prompts. It verifies
raw retrieval as a positive control, runs the current CLI with unchanged source
signatures and default tiers, then verifies:

- Legacy content-hashed chunk IDs disappear and all three tiers are rebuilt.
- Full `hev trace --json`, dashboard detail, list, stats and facet responses
  contain no original specimens, including JSON-escaped copies in tool text.
- Each original is submitted to real `hev query --json` and dashboard search.
  Searches prepend `credential` to provide tokenizer context for punctuation-only
  credentials. Semantic hits may still occur; returned content must be scrubbed.
  The dashboard's echo of the submitted `q` is excluded from archive assertions.
- Every distinct archived marker is retrievable through real hybrid query.
- A second scan skips both unchanged source units and preserves the salt;
  source JSONL bytes remain unchanged.
- An isolated foreground daemon persists exactly the independently computed
  per-rule counts; `hev s` displays every count.
- Explicit opt-out after migration stores raw content again; re-enable cleans
  it up on the next scan.

Optional Playwright checks render both sessions in the actual embedded dashboard
and check visible text for originals and markers. Install Playwright outside the
repository and use an absolute script path. Point browser caches outside the
isolated HOME if necessary:

```sh
browser_dir=$(mktemp -d)
npm install --prefix "$browser_dir" playwright
"$browser_dir/node_modules/.bin/playwright" install chromium --only-shell
NODE_PATH="$browser_dir/node_modules" \
PLAYWRIGHT_BROWSERS_PATH=/path/to/browser/cache \
HEV_REDACTION_BROWSER_SCRIPT="$PWD/scripts/test-redaction-browser.cjs" \
HEV_REDACTION_EVIDENCE_DIR=/path/to/evidence \
HEV_REDACTION_LIVE_CONFIG=/path/to/target.toml \
HEV_REDACTION_LEGACY_BINARY=/path/to/legacy/hev \
  go test ./cmd/hev -run '^TestLiveRedactionAcceptance$' -count=1 -timeout 45m -v
```

Afterward remove only your acceptance Compose project and volumes using the same
project and file arguments with `down --volumes`.

## Recorded validation

On 2026-10-01, final validation of the Codex argument fix (commit `402bfa5`)
with the final fixture harness in `5130ee6` used the actual pre-redaction
`e57a7a8` binary and manually seeded raw session
summaries. Both fixture sessions had 648 turns, covering every specimen in each
payload position. The final pgvector run passed in 225.32 seconds; the hosted
native GCP Turbopuffer run passed in 809.06 seconds.

| Check | Local pgvector | Hosted Turbopuffer |
| --- | --- | --- |
| Endpoint | published CE gateway 0.7.3 | native GCP us-central1 Turbopuffer API |
| Embedder | real CPU BGE-small sidecar | real Qwen3 embedding 8B |
| Specimens / parsed payloads | 216 / 1,296 | 216 / 1,296 |
| Searchable distinct keyed markers | 216 | 216 |
| `hev s` replacements / winning rules | 1,296 / 203 | 1,296 / 203 |
| Legacy raw IDs, prompts, blocks and summaries removed | passed | passed |
| Raw originals absent from CLI and dashboard retrieval | passed | passed |
| Chromium rendered both harness sessions | passed | passed |
| Opt-out, re-enable, unchanged signatures and raw sources | passed | passed |

Per-rule counts and browser reports are in [redaction evidence](evidence/redaction).
The fixed fixture salt is intentionally reproducible and is not an installation
credential. Overlap priority explains why 216 specimens have 203 winning rule
names. Browser assertions inspect rendered text; API assertions cover the full
prompt, tool-use and tool-result payloads, including collapsed browser content.

Local `go test -race -count=1 -skip '^TestAgainstRealTranscripts$' ./...`, build,
vet, formatting, tidy and diff checks passed. Only the existing scan of the
mini's private transcript corpus was excluded locally; CI runs the full race
suite without a skip. The live fixture is opt-in and is not run by ordinary CI.
The named **ci / go** workflow passed on `5130ee6`:
[CI run](https://github.com/hev/kit/actions/runs/36931012654).
The PR records the final documentation head's workflow result separately.

One qualifying specimen per rule does not prove all token variants. No live
Claude summary-model call was made: existing request-boundary tests use a fake
Claude command, while the live run verifies stored legacy summary removal.
Interrupted migrations, missing sources and ambiguous ownership have stateful
store tests rather than injected live-service failures. Earlier attempts exposed
and corrected a missing facet parameter and the query-echo assertion; the hosted
provider returned one transient embedding 429, which failed that attempt. A
later final AWS Layer run failed on a query 504; a serial retry failed when the
gateway became unavailable (503), including fixture cleanup. Its leftover
namespace was verified as owned by this fixture and removed through the native
AWS Turbopuffer endpoint; the GCP endpoint confirmed it was absent there. The
final hosted attempts therefore used native Turbopuffer endpoints.
A native 8B attempt also failed on the shared embedding provider's 429.
A Qwen3 0.6B trial was rejected as unsupported in that AWS region.
The passing final retry used kit's native GCP endpoint with the default 8B model.
Earlier AWS Layer gateway runs passed before the final argument-delimiter
refinement; they do not establish final-code acceptance through that gateway. The
Postgres tokenizer rejected a punctuation-only query, so original searches use
the documented `credential` context. These failures were not counted as passes.

The initial corpus exposed Codex tool arguments stored as escaped JSON strings:
provider assignment regexes missed their delimiters. Decoding those argument
values before scrubbing fixes the parser cases. Policy `gitleaks-8.24.3-kit-2`
invalidates previous migration completion, including archives written under the
first scrubbing policy. This handles JSON string arguments, not arbitrary
base64, encrypted, split or unsupported secrets. Source files remain raw.
