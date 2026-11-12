#!/usr/bin/env bash
# Run only on a credentialed machine; creates billable resources.
# Usage: BOAT_API_KEY, BOAT_SSH_PUBLIC_KEY, HEVLAYER_API_KEY,
# HEVLAYER_ENDPOINT and a loaded SSH agent; optional OPENAI_API_KEY.
# scripts/boat-live-demo.sh [--template NAME] [--delete-template]
# A new template is built unless --template names an existing one.
set +x
set -euo pipefail
exec python3 "$(dirname "$0")/boat_live_demo.py" "$@"
