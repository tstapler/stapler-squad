#!/usr/bin/env bash
# otel-auto-loadgen.sh — fixed, repeatable workload for Story 3.1.3's pprof
# comparison (project_plans/go-auto-instrumentation/implementation/plan.md).
#
# Creates COUNT lightweight sessions (program=bash, session_type=DIRECTORY —
# no worktree checkout, no git branch creation, no real agent spawn) against
# a running stapler-squad(-otel) instance on PORT, then repeatedly lists all
# sessions and polls GetSession/GetVCSStatus for each created session for
# DURATION seconds, exercising the same request shapes as Spike E
# (spike-verdicts.md) and this repo's session-poller hot paths (GetStatus,
# ReviewQueuePoller.checkSession, CircularBuffer, GitWorktree.IsDirty).
# Always deletes every session it created, even on failure/interrupt.
#
# Usage:
#   PORT=62871 DURATION=30 COUNT=5 ./scripts/otel-auto-loadgen.sh
#
# Env vars (all optional):
#   PORT             - target instance port (default: 62871)
#   DURATION         - seconds to drive the poll loop (default: 30)
#   COUNT            - number of DIRECTORY-type sessions to create (default: 5)
#   WORKTREE_COUNT   - number of additional SESSION_TYPE_NEW_WORKTREE sessions to
#                      create (default: 0). DIRECTORY sessions never construct a
#                      session/git.GitWorktree, so they cannot exercise
#                      GitWorktree.IsDirty; set this to also create real
#                      worktree-backed sessions (each on its own throwaway
#                      branch) so that hot path is actually driven. Each such
#                      session performs a real `git worktree add` under
#                      ~/.stapler-squad/worktrees/, cleaned up (worktree
#                      removed) by DeleteSession on exit.
#   WORKSPACE_PATH   - path passed as CreateSessionRequest.path (default: cwd)
set -euo pipefail

PORT="${PORT:-62871}"
DURATION="${DURATION:-30}"
COUNT="${COUNT:-5}"
WORKTREE_COUNT="${WORKTREE_COUNT:-0}"
WORKSPACE_PATH="${WORKSPACE_PATH:-$(pwd)}"
BASE="http://localhost:${PORT}/api/session.v1.SessionService"

curl_json() {
  # $1 = RPC method name, $2 = JSON body
  curl -s -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" \
    -d "$2" "${BASE}/$1"
}

ids=()

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  local id
  for id in "${ids[@]+"${ids[@]}"}"; do
    curl_json DeleteSession "{\"id\":\"${id}\",\"force\":true}" >/dev/null 2>&1 || true
  done
  exit "$status"
}
trap cleanup EXIT INT TERM

echo "otel-auto-loadgen: creating ${COUNT} session(s) on port ${PORT} (path=${WORKSPACE_PATH})"
for i in $(seq 1 "${COUNT}"); do
  resp="$(curl_json CreateSession "{\"title\":\"loadgen-${i}-$$\",\"path\":\"${WORKSPACE_PATH}\",\"program\":\"bash\",\"sessionType\":\"SESSION_TYPE_DIRECTORY\"}")" || true
  id="$(printf '%s' "${resp}" | jq -r '.session.id // empty' 2>/dev/null || true)"
  if [ -z "${id}" ]; then
    echo "WARN: failed to create session ${i}: ${resp}" >&2
    continue
  fi
  ids+=("${id}")
done

if [ "${WORKTREE_COUNT}" -gt 0 ]; then
  for i in $(seq 1 "${WORKTREE_COUNT}"); do
    branch="loadgen-wt-$$-${i}"
    resp="$(curl_json CreateSession "{\"title\":\"loadgen-wt-${i}-$$\",\"path\":\"${WORKSPACE_PATH}\",\"branch\":\"${branch}\",\"program\":\"bash\",\"sessionType\":\"SESSION_TYPE_NEW_WORKTREE\"}")" || true
    id="$(printf '%s' "${resp}" | jq -r '.session.id // empty' 2>/dev/null || true)"
    if [ -z "${id}" ]; then
      echo "WARN: failed to create worktree session ${i}: ${resp}" >&2
      continue
    fi
    ids+=("${id}")
  done
fi

echo "otel-auto-loadgen: created ${#ids[@]} session(s); driving load for ${DURATION}s"
end=$((SECONDS + DURATION))
rounds=0
while [ "${SECONDS}" -lt "${end}" ]; do
  curl_json ListSessions '{}' >/dev/null || true
  for id in "${ids[@]+"${ids[@]}"}"; do
    curl_json GetSession "{\"id\":\"${id}\"}" >/dev/null || true
    curl_json GetVCSStatus "{\"id\":\"${id}\"}" >/dev/null || true
  done
  rounds=$((rounds + 1))
  sleep 0.5
done

echo "otel-auto-loadgen: completed ${rounds} poll round(s) across ${#ids[@]} session(s)"
