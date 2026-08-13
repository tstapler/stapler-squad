#!/usr/bin/env bash
#
# open-code-proxy.sh - Proxy for open-code with Stapler Squad permissions
#
# This script intercepts calls to open-code and routes them through
# ssq-hooks proxy to ensure proper permission checks.
#

set -euo pipefail

# Pass all arguments to ssq-hooks proxy, which outputs the command to run.
# Use exec to replace this process with the proxied command directly,
# avoiding eval and its shell injection risks.
exec ssq-hooks proxy --exec -- open-code "$@"
