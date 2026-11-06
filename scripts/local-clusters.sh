#!/bin/sh
# Purpose: Start the explicitly scoped Phase 1A fixture lifecycle tool.
# Used by: Opt-in integration and packaged validation.
# Responsibilities: Delegate up/down to the ownership-aware local tool.
# Important: Creating fixtures on primary A requires explicit user authorization.
set -eu
exec python3 "$(dirname "$0")/local-clusters.py" "$@"
