#!/bin/bash
# Times hev traces commands on a real file archive, with ripgrep and without.
# usage: speed.sh <hev> <home>
HEV=$1 H=$2
t() { # label, engine, args...
  local label=$1 eng=$2; shift 2
  local best=999
  for i in 1 2 3; do
    s=$(date +%s.%N)
    env -u HEVLAYER_API_KEY_REF HOME=$H HEV_CONFIG=$H/.hev/config.toml HEV_GREP=$eng "$HEV" "$@" >/dev/null 2>&1
    e=$(date +%s.%N)
    best=$(python3 -c "print(min($best, $e-$s))")
  done
  printf "| %s | %s | %.2f s |\n" "$label" "$eng" "$best"
}
for eng in "" go; do
  e=${eng:-rg}
  t "list the newest 20 sessions" "$eng" traces ls --since 9999d
  t "search, one word (\`preflight\`)" "$eng" traces search preflight
  t "search, a question (\`why did the deploy time out\`)" "$eng" traces search "why did the deploy time out"
  t "search, three phrasings (\`--also\` ×2)" "$eng" traces search "deploy timeout" --also "helm rollout" --also "deadline exceeded"
  t "search, \`--since 7d\`" "$eng" traces search --since 7d "preflight"
  t "search, \`--harness codex\`" "$eng" traces search --harness codex "preflight"
  t "search, \`--regex\`" "$eng" traces search --regex '(?i)exit (code|status) 1[0-9][0-9]'
done
