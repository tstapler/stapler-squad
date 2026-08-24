#!/usr/bin/env bash
# Collector Smoke Test (default) / Suppression Smoke Test (--suppression) for
# the opt-in otelc auto-instrumentation build — turns Spike B.2's and Spike
# D's one-time manual checks into a repeatable script, so a future otelc
# upgrade can neither silently stop weaving (default mode) nor silently
# start exporting when telemetry is off (--suppression). Runs its own OTLP
# collector (Docker, :4317/:4318) and its own stapler-squad-otel process(es)
# under STAPLER_SQUAD_INSTANCE=claude-otel-smoke on the manual dev port block
# (CLAUDE.md) — everything spawned is torn down by a trap on every exit path.
# Background: project_plans/go-auto-instrumentation/implementation/spike-verdicts.md
# (Spike B.2, Spike D).
#
# Usage:
#   ./scripts/otel-auto-smoke.sh [binary_path]                     # Story 2.2.1
#   ./scripts/otel-auto-smoke.sh [binary_path] --suppression       # Story 2.2.2
#   ./scripts/otel-auto-smoke.sh --suppression --self-test         # proves the
#     zero-span assertion discriminates by forcing tracing ON into the
#     "suppression" leg; expected to FAIL with a specific message.
#   ./scripts/otel-auto-smoke.sh [binary_path] --with-subprocess   # Story 5.1.3
#     additionally asserts a subprocess.command=git span from the
#     executor/safeexec hook (instrumentation/otelc/safeexec) — created by
#     driving a real CreateSession + GetVCSStatus round trip against this
#     checkout, both of which run `git` through safeexec.CommandContext
#     (confirmed empirically, 2026-08-22: CreateSession's own git-detection
#     produces a subprocess.command=git span with arg_count=5; GetVCSStatus's
#     provider.GetStatus() produces one with arg_count=4, matching
#     session/git's `git -C <path> status --porcelain` shape — see
#     spike-verdicts.md, Spike E addendum).
set -euo pipefail

PORT=62871
INSTANCE="claude-otel-smoke"
COLLECTOR_CONTAINER="otelcol-smoke"
BATCH_WAIT=8 # default BatchSpanProcessor export interval (Spike B/D)

BINARY="./stapler-squad-otel"
MODE="default"
SELF_TEST="false"
WITH_SUBPROCESS="false"
for arg in "$@"; do
  case "$arg" in
    --suppression) MODE="suppression" ;;
    --self-test) SELF_TEST="true" ;;
    --with-subprocess) WITH_SUBPROCESS="true" ;;
    -*)
      echo "✗ unknown flag: $arg" >&2
      exit 1
      ;;
    *) BINARY="$arg" ;;
  esac
done

if [ "$MODE" != "suppression" ] && [ "$SELF_TEST" = "true" ]; then
  echo "✗ --self-test only applies to --suppression" >&2
  exit 1
fi

if [ "$MODE" = "suppression" ] && [ "$WITH_SUBPROCESS" = "true" ]; then
  echo "✗ --with-subprocess only applies to the default (non-suppression) mode" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "✗ docker not found on PATH — required to run the local OTLP collector for this smoke test." >&2
  exit 1
fi

if [ ! -x "$BINARY" ]; then
  echo "✗ $BINARY not found or not executable. Build it first: make build-otel-auto" >&2
  exit 1
fi

APP_PID=""
COLLECTOR_CONFIG_FILE=""
COLLECTOR_STARTED="false"

# shellcheck disable=SC2329,SC2317  # invoked indirectly via `trap ... EXIT` below
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [ -n "$APP_PID" ]; then
    kill "$APP_PID" >/dev/null 2>&1 || true
    wait "$APP_PID" 2>/dev/null || true
  fi
  if [ "$COLLECTOR_STARTED" = "true" ]; then
    docker rm -f "$COLLECTOR_CONTAINER" >/dev/null 2>&1 || true
  fi
  [ -n "$COLLECTOR_CONFIG_FILE" ] && rm -f "$COLLECTOR_CONFIG_FILE"
  exit "$status"
}
trap cleanup EXIT INT TERM

start_collector() {
  docker rm -f "$COLLECTOR_CONTAINER" >/dev/null 2>&1 || true
  COLLECTOR_CONFIG_FILE="$(mktemp)"
  cat >"$COLLECTOR_CONFIG_FILE" <<'EOF'
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
EOF
  # otelcol-contrib's image runs as a non-root user; mktemp's default 0600
  # mode is unreadable to it once bind-mounted in, so the container exits
  # immediately with "permission denied" reading its own config.
  chmod 644 "$COLLECTOR_CONFIG_FILE"
  docker run -d --name "$COLLECTOR_CONTAINER" -p 127.0.0.1:4317:4317 -p 127.0.0.1:4318:4318 \
    -v "$COLLECTOR_CONFIG_FILE:/etc/otelcol-contrib/config.yaml:ro" \
    otel/opentelemetry-collector-contrib:latest >/dev/null
  COLLECTOR_STARTED="true"
  for _ in $(seq 1 30); do
    docker logs "$COLLECTOR_CONTAINER" 2>&1 | grep -q "Everything is ready" && return 0
    sleep 1
  done
  echo "✗ collector did not become ready in time" >&2
  exit 1
}

# start_app [env=val ...] — always uses the manual port block + isolated instance.
start_app() {
  env "$@" PORT="$PORT" STAPLER_SQUAD_INSTANCE="$INSTANCE" \
    "$BINARY" --tmux-keep-server >/tmp/otel-auto-smoke-app.log 2>&1 &
  APP_PID=$!
  for _ in $(seq 1 60); do
    curl -sf "http://localhost:$PORT/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  echo "✗ $BINARY did not become healthy on :$PORT" >&2
  cat /tmp/otel-auto-smoke-app.log >&2
  exit 1
}

stop_app() {
  if [ -n "$APP_PID" ]; then
    kill "$APP_PID" >/dev/null 2>&1 || true
    wait "$APP_PID" 2>/dev/null || true
    APP_PID=""
  fi
}

drive_request() {
  curl -sf "http://localhost:$PORT/" >/dev/null
  curl -sf "http://localhost:$PORT/api/session.v1.SessionService/ListSessions" \
    -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" \
    -d '{}' >/dev/null
}

# trigger_subprocess_check — Story 5.1.3's driven trigger for the
# executor/safeexec hook. Creates a throwaway session against this checkout
# (always a real git repo) and immediately queries its VCS status, then
# deletes the session again. Both CreateSession (its own git-detection step)
# and GetVCSStatus (provider.GetStatus(), which shares session/git's
# runGitCommand -> safeexec.CommandContext choke point) run a real `git`
# subprocess, giving the hook something to fire on. Best-effort throughout
# (`|| true`): a failed create/status/delete here should surface as "no
# subprocess.command span observed" below, not as an unrelated curl error.
trigger_subprocess_check() {
  local repo_root resp sid
  repo_root="$(pwd)"
  resp="$(curl -sf -X POST "http://localhost:$PORT/api/session.v1.SessionService/CreateSession" \
    -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" \
    -d "{\"title\":\"otel-auto-smoke-subprocess\",\"path\":\"$repo_root\",\"program\":\"bash\",\"sessionType\":\"SESSION_TYPE_DIRECTORY\"}")" || true
  sid="$(printf '%s' "${resp:-}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)"
  if [ -n "${sid:-}" ]; then
    curl -sf -X POST "http://localhost:$PORT/api/session.v1.SessionService/GetVCSStatus" \
      -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" \
      -d "{\"id\":\"$sid\"}" >/dev/null || true
    curl -sf -X POST "http://localhost:$PORT/api/session.v1.SessionService/DeleteSession" \
      -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" \
      -d "{\"id\":\"$sid\"}" >/dev/null || true
  fi
}

collector_line_count() {
  docker logs "$COLLECTOR_CONTAINER" 2>&1 | wc -l
}

# count_db_system_since <line-count-checkpoint> — the "truncate without
# restarting the collector" requirement (Story 2.2.2): Docker's log driver
# has no truncate primitive, so a saved line-count offset stands in for it,
# matching Spike D's own methodology note.
count_db_system_since() {
  local checkpoint="$1"
  docker logs "$COLLECTOR_CONTAINER" 2>&1 | tail -n "+$((checkpoint + 1))" | grep -c "db.system" || true
}

# count_subprocess_git_since <line-count-checkpoint> — same technique as
# count_db_system_since, for the subprocess.command=git span the
# executor/safeexec hook emits (Story 5.1.3).
count_subprocess_git_since() {
  local checkpoint="$1"
  docker logs "$COLLECTOR_CONTAINER" 2>&1 | tail -n "+$((checkpoint + 1))" | grep -c "subprocess.command: Str(git)" || true
}

census_since() {
  local checkpoint="$1"
  docker logs "$COLLECTOR_CONTAINER" 2>&1 | tail -n "+$((checkpoint + 1))" | grep -B2 -A6 "db.system" || true
}

subprocess_census_since() {
  local checkpoint="$1"
  docker logs "$COLLECTOR_CONTAINER" 2>&1 | tail -n "+$((checkpoint + 1))" | grep -B7 -A1 "subprocess.command: Str(git)" || true
}

ON_ENV=(OTEL_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc)

run_default_mode() {
  start_collector
  local checkpoint
  checkpoint="$(collector_line_count)"
  start_app "${ON_ENV[@]}"
  drive_request
  if [ "$WITH_SUBPROCESS" = "true" ]; then
    trigger_subprocess_check
  fi
  sleep "$BATCH_WAIT"
  local census
  census="$(census_since "$checkpoint")"
  local subprocess_census=""
  if [ "$WITH_SUBPROCESS" = "true" ]; then
    subprocess_census="$(subprocess_census_since "$checkpoint")"
  fi
  stop_app

  if [ -z "$census" ]; then
    echo "no db.system span observed — binary is not auto-instrumented" >&2
    exit 1
  fi

  echo "Span Census (db.system spans observed for $BINARY):"
  echo "$census"

  if [ "$WITH_SUBPROCESS" = "true" ]; then
    if [ -z "$subprocess_census" ]; then
      echo "no subprocess.command=git span observed — executor/safeexec hook did not fire" >&2
      exit 1
    fi
    echo "Span Census (subprocess.command=git spans observed for $BINARY):"
    echo "$subprocess_census"
  fi

  echo "✅ otel-auto-smoke: db.system span(s) observed for $BINARY"
  if [ "$WITH_SUBPROCESS" = "true" ]; then
    echo "✅ otel-auto-smoke --with-subprocess: subprocess.command=git span(s) observed for $BINARY"
  fi
}

run_suppression_mode() {
  start_collector

  # (1) Collector Liveness Check — proven-live before any zero-span assertion.
  local checkpoint live_count
  checkpoint="$(collector_line_count)"
  start_app "${ON_ENV[@]}"
  drive_request
  sleep "$BATCH_WAIT"
  live_count="$(count_db_system_since "$checkpoint")"
  stop_app
  if [ "$live_count" -lt 1 ]; then
    echo "collector not live — suppression result would be meaningless" >&2
    exit 1
  fi
  echo "Collector Liveness Check: PASS ($live_count db.system span(s) observed)"

  # (2) Truncate (checkpoint) without restarting the collector.
  checkpoint="$(collector_line_count)"

  # (3) Suppression leg: tracing-off Run Recipe is "OTEL_ENABLED/DD_TRACE_ENABLED
  # unset" per spike-verdicts.md's Spike D finding (Spike D.2's Exporter Toggle
  # remedy was N/A — no leak observed) — NOT the generic "OTEL_ENABLED=false"
  # some doc templates assume; kept in sync with
  # .claude/docs/opentelemetry-auto-instrumentation.md's Run Recipe.
  # --self-test forces tracing ON here instead, to prove the zero-span
  # assertion below actually discriminates.
  local suppressed_count
  if [ "$SELF_TEST" = "true" ]; then
    start_app "${ON_ENV[@]}"
  else
    start_app
  fi
  drive_request
  sleep "$BATCH_WAIT"
  suppressed_count="$(count_db_system_since "$checkpoint")"
  stop_app

  # (4) Positive control — runs unconditionally, PASS or FAIL, so a
  # zero-span suppression result is never confused with a dead collector.
  checkpoint="$(collector_line_count)"
  local control_count
  start_app "${ON_ENV[@]}"
  drive_request
  sleep "$BATCH_WAIT"
  control_count="$(count_db_system_since "$checkpoint")"
  stop_app

  # (5) Print both (all three) span counts.
  echo "Span counts — liveness: $live_count, suppression: $suppressed_count, positive control: $control_count"

  if [ "$control_count" -lt 1 ]; then
    echo "positive control failed — collector died mid-test, suppression result is unverified" >&2
    exit 1
  fi

  if [ "$suppressed_count" -gt 0 ]; then
    echo "spans exported while telemetry disabled — suppression is broken" >&2
    exit 1
  fi

  echo "✅ otel-auto-smoke --suppression: suppression verified for $BINARY"
}

if [ "$MODE" = "suppression" ]; then
  run_suppression_mode
else
  run_default_mode
fi
