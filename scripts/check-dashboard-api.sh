#!/usr/bin/env bash
# Read-only acceptance checks against the local Go fixture.
set -euo pipefail
base=${HEV_DASHBOARD_URL:-http://127.0.0.1:18787}
corpus=$(curl --fail --silent "$base/api/sessions?window=all")
printf '%s\n' "$corpus" | jq '[.sessions[].id] | length, (unique|length)'
printf '%s\n' "$corpus" | jq -e 'all(.sessions[]; has("first_prompt_short") and (has("first_prompt")|not) and (has("prompt_ts")|not) and (has("tool_names")|not))'
# Recover detail-only fields for independent jq filter/coverage checks.
corpus=$(printf '%s\n' "$corpus" | jq -c '.sessions[]' | while IFS= read -r row; do
  id=$(printf '%s\n' "$row" | jq -r '.id|@uri')
  detail=$(curl --fail --silent "$base/api/session/$id")
  printf '%s\n' "$row" | jq --argjson detail "$detail" '. + {prompt_ts:$detail.prompt_ts,tool_names:$detail.tool_names}'
done | jq -s '{sessions:.}')
with=$(printf '%s\n' "$corpus" | jq '[.sessions[]|select(.prompt_ts|length>0)]|length')
stats_with=$(curl --fail --silent "$base/api/stats?window=all" | jq '.prompt_coverage.with')
test "$with" = "$stats_with"
printf 'prompt coverage matches detail: %s\n' "$with"
check() {
  local query=$1 predicate=$2 expected actual stats
  expected=$(printf '%s\n' "$corpus" | jq "[.sessions[]|select($predicate)]|length")
  actual=$(curl --fail --silent "$base/api/sessions?window=all&$query" | jq '.sessions|length')
  stats=$(curl --fail --silent "$base/api/stats?window=all&$query" | jq '.traces')
  test "$expected" = "$actual"
  test "$expected" = "$stats"
  printf '%s: jq=%s sessions=%s stats=%s\n' "$query" "$expected" "$actual" "$stats"
}
check 'project=alpha&project=beta&model=model-a&tool=Bash&cost_min=1' '((.project=="alpha" or .project=="beta") and .model=="model-a" and (.tool_names|index("Bash")) and .cost>=1)'
check 'harness=claude_code&host=laptop&tools_min=2&tools_max=5&tokens_min=200&tokens_max=500&cost_min=2&cost_max=5&wall_min=2&wall_max=5' '(.harness=="claude_code" and .host=="laptop" and .tools>=2 and .tools<=5 and .total_tokens>=200 and .total_tokens<=500 and .cost>=2 and .cost<=5 and .wall_ms>=120000 and .wall_ms<=300000)'
check 'poor=true' '(.eval.poor==true)'
check 'mark_accuracy_max=2&role=reviewer&instance=nightly' '(.eval!=null and .eval.marks.accuracy<=2 and .eval.role=="reviewer" and .eval.instance=="nightly")'
curl --fail --silent "$base/api/search?q=preflight&project=alpha" | jq -e '[.sessions[]|(.project=="alpha" and (.snippet|length)>0 and (.turn_uuid|length)>0)]|length>0 and all'
curl --fail --silent "$base/api/search?q=Distinct%20evaluator%20phrase&poor=true" | jq -e '.sessions|length==1 and .[0].source=="eval" and .[0].eval.poor==true'
printf 'PASS: transcript and evaluator snippets\n'
