#!/bin/sh
# Build once, then exercise every hev command mentioned in README/MISSION/help.
set -eu
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
go build -o "$scratch/hev" ./cmd/hev
check() { "$scratch/hev" "$@" --help > /dev/null; }
check
check up
check d
check s
check down
check dispatch
check dispatch logs
check dispatch show
check dispatch say
check dispatch amend
check peers
check peers read
check peers send
check board
check traces
check traces search
check traces show
check traces ls
# Legacy commands remain callable, including their offline help handlers.
for verb in apply get describe controller logs watch wait api-resources; do
 check "$verb"
done
printf 'Factory and legacy command help checks passed.\n'
