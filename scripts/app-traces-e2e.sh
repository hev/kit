#!/usr/bin/env bash
# End-to-end check of app capture with no Layer key: hev collect on the file
# archive, no Docker, nothing hosted. The check itself is
# scripts/app-traces-e2e.mjs, which runs the same on Windows:
#
#   node scripts/app-traces-e2e.mjs      # PowerShell or cmd on Windows
#
# Needs Go, Node 22+ and npm. See the header of app-traces-e2e.mjs.
set -euo pipefail
exec node "$(cd "$(dirname "$0")" && pwd)/app-traces-e2e.mjs" "$@"
