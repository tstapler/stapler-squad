#!/usr/bin/env bash
# Guarded manual-instance runner: the only sanctioned way for a worker to try a
# running build (CLAUDE.md "Manual/interactive testing without touching the live
# deployed instance"). It never touches the live service, its ports, its state
# directory or its tmux server.
#
# Usage:
#   scripts/verify-manual-instance.sh --check <N>   dry run: print plan, run the port precheck, start nothing
#   scripts/verify-manual-instance.sh <N>            build, start, wait for health, stay up until Ctrl-C/TERM
#
# Slot N (1-5) maps to:
#   PORT        62871 + 2(N-1)
#   --remote-port 62872 + 2(N-1)
#   binary      ~/.stapler-squad/manual-builds/manual-N/stapler-squad
#   state       STAPLER_SQUAD_INSTANCE=claude-manual-test-N
#   tmux        STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-N (the only socket torn down)

set -euo pipefail

readonly BASE_PORT=62871
readonly MAX_SLOT=5
readonly LIVE_PORTS=(8543 8444)
readonly HEALTH_TIMEOUT_SECONDS=60

usage() {
	sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
}

die() {
	echo "verify-manual-instance: $*" >&2
	exit 1
}

check_only=false
slot=""
for arg in "$@"; do
	case "$arg" in
	--check) check_only=true ;;
	-h | --help)
		usage
		exit 0
		;;
	-*) die "unknown flag: $arg" ;;
	*)
		[[ -z "$slot" ]] || die "only one slot argument allowed"
		slot="$arg"
		;;
	esac
done

[[ "$slot" =~ ^[1-9][0-9]*$ ]] || {
	usage >&2
	die "slot N must be a positive integer"
}
((slot >= 1 && slot <= MAX_SLOT)) || die "slot N must be 1-$MAX_SLOT (the reserved port block holds $MAX_SLOT slots)"

port=$((BASE_PORT + 2 * (slot - 1)))
remote_port=$((port + 1))
instance="claude-manual-test-${slot}"
tmux_socket="ssq-manual-${slot}"
build_dir="${HOME}/.stapler-squad/manual-builds/manual-${slot}"
binary="${build_dir}/stapler-squad"

for live in "${LIVE_PORTS[@]}"; do
	if [[ "$port" == "$live" || "$remote_port" == "$live" ]]; then
		die "computed port collides with live port $live; refusing"
	fi
done
[[ "$tmux_socket" =~ ^ssq-manual-[1-9]$ ]] || die "tmux socket '$tmux_socket' is not a manual-instance socket; refusing"

port_holders() {
	ss -ltnp 2>/dev/null | grep -E ":(${port}|${remote_port})\b" || true
}

precheck_ports() {
	local holders
	holders="$(port_holders)"
	if [[ -n "$holders" ]]; then
		echo "$holders" >&2
		die "port $port or $remote_port is already in use (stale manual instance?); stop it before continuing"
	fi
}

print_plan() {
	cat <<EOF
slot:                       ${slot}
PORT:                       ${port}
--remote-port:              ${remote_port}
binary:                     ${binary}
STAPLER_SQUAD_INSTANCE:     ${instance}
STAPLER_SQUAD_TMUX_SOCKET:  ${tmux_socket}
state dir:                  ${HOME}/.stapler-squad/instances/${instance}/
teardown:                   kill own PID, then tmux -L ${tmux_socket} kill-server (only)
EOF
}

command -v ss >/dev/null 2>&1 || die "ss not found; cannot run the port precheck"

if $check_only; then
	echo "[check] dry run: nothing is built or started"
	print_plan
	precheck_ports
	echo "[check] ports ${port} and ${remote_port} are free"
	exit 0
fi

command -v go >/dev/null 2>&1 || die "go not found"
command -v tmux >/dev/null 2>&1 || die "tmux not found"
command -v curl >/dev/null 2>&1 || die "curl not found"

print_plan
precheck_ports

pid=""
# shellcheck disable=SC2329 # invoked via trap
teardown() {
	local rc=$?
	trap - EXIT INT TERM
	if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	tmux -L "$tmux_socket" kill-server 2>/dev/null || true
	exit "$rc"
}
trap teardown EXIT INT TERM

mkdir -p "$build_dir"
go build -o "$binary" .

PORT="$port" \
	STAPLER_SQUAD_INSTANCE="$instance" \
	STAPLER_SQUAD_TMUX_SOCKET="$tmux_socket" \
	"$binary" --tmux-keep-server --remote-port "$remote_port" &
pid=$!

url="http://localhost:${port}/"
for ((i = 0; i < HEALTH_TIMEOUT_SECONDS; i++)); do
	kill -0 "$pid" 2>/dev/null || die "instance exited before becoming healthy"
	if curl -fsS -o /dev/null "$url"; then
		echo "ready: ${url} (pid ${pid}); Ctrl-C to stop and tear down tmux socket ${tmux_socket}"
		wait "$pid"
		exit 0
	fi
	sleep 1
done
die "instance did not answer ${url} within ${HEALTH_TIMEOUT_SECONDS}s"
