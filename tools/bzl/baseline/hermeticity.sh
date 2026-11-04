#!/usr/bin/env bash
# Task 0.1j: hermeticity baseline (T7 inputs H1, H2) on the make path.
#
#   H1  distinct host executables (successful execve outside the workspace and
#       the pinned toolchain dirs: GOROOT, GOMODCACHE, the repo's bun/shared
#       node_modules) and distinct non-loopback network destinations, from
#       `strace -f -e trace=execve,connect` over a fresh-clone `make build`.
#       "UNMEASURED: <reason>" when strace is missing or cannot trace; never
#       estimated.
#   H2  fresh-clone `make build` under `env -i PATH=/usr/bin:/bin`; boolean.
#   H3  Bazel only (Task 7.5g); not computed here.
#   H4  rows in tools/bzl/hermetic-downgrades.txt (0 when the file is absent).
#
# Heavy (a full cold build twice): refuses while the 1-minute load average
# exceeds 2.0 and has no override. --dry-run and --parse-strace are cheap.
#
# usage: hermeticity.sh --system make [--dry-run] [--out <json>]
#        hermeticity.sh --parse-strace <strace.log> [--workspace <dir>]   (self-test of the H1 parser)
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

SYSTEM="" DRY=0 OUT="" PARSE="" WS=""
while (($#)); do
  case "$1" in
    --system) SYSTEM="$2"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    --out) OUT="$2"; shift 2 ;;
    --parse-strace) PARSE="$2"; shift 2 ;;
    --workspace) WS="$2"; shift 2 ;;
    -h|--help) sed -n '2,22p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
need jq

# h1_parse <strace-log> <workspace> <pinned-dir>...: prints JSON.
h1_parse() {
  local log="$1" ws="$2"; shift 2
  local pinned
  pinned="$(printf '%s\n' "$@" | grep -v '^$' | paste -sd'|' -)"
  local execs conns
  # successful execve: path is the first quoted string, return value 0.
  execs="$(grep -E '^[0-9]* *execve\(' "$log" | grep -E '= 0$' \
    | sed -E 's/^[0-9]* *execve\("([^"]+)".*/\1/' | sort -u || true)"
  local host_execs
  host_execs="$(awk -v ws="$ws" -v pin="$pinned" '
    { if (index($0, ws "/")==1) next
      n=split(pin, p, "|"); skip=0
      for (i=1;i<=n;i++) if (p[i]!="" && index($0, p[i] "/")==1) skip=1
      if (!skip) print }' <<<"$execs")"
  conns="$(grep -E 'connect\(' "$log" | grep -E 'sa_family=AF_INET6?' | grep -vE '= -1 (ENOENT|EACCES)' \
    | sed -nE 's/.*sin6?_port=htons\(([0-9]+)\).*(inet_addr\("([^"]+)"\)|inet_pton\(AF_INET6, "([^"]+)"[^)]*\)).*/\3\4:\1/p' \
    | grep -vE '^(127\.|::1:|0\.0\.0\.0:)' | sort -u || true)"
  jq -n --arg e "$host_execs" --arg c "$conns" '
    {h1_host_executables:($e|split("\n")|map(select(length>0))|length),
     h1_host_executable_list:($e|split("\n")|map(select(length>0))),
     h1_network_destinations:($c|split("\n")|map(select(length>0))|length),
     h1_network_destination_list:($c|split("\n")|map(select(length>0)))}'
}

if [[ -n "$PARSE" ]]; then
  h1_parse "$PARSE" "${WS:-/nonexistent-ws}" /usr/local/go
  exit 0
fi

[[ "$SYSTEM" == make ]] || { [[ "$SYSTEM" == bazel ]] && die "bazel mode is run by Task 7.5g (needs the Bazel build)"; die "--system make|bazel required"; }

if ((DRY)); then
  jq -n --arg l "$(loadavg1)" '{dry_run:true, system:"make", loadavg1_now:($l|tonumber), max_loadavg1:2.0,
    steps:["require_quiet_host","git clone HEAD to short tmp dir",
           "H1: strace -f -e trace=execve,connect -o log -- make build",
           "H2: env -i PATH=/usr/bin:/bin make build","write JSON"]}'
  exit 0
fi

need git; need make; need go
require_quiet_host

D="$(mktemp -d "${TMPDIR:-/tmp}/bzlh.XXXXXX")"
trap 'rm -rf "$D"' EXIT
fresh_clone() { git clone --quiet --no-hardlinks "$BZL_REPO_ROOT" "$1"; }

# ---- H1 ----
h1='{"h1_host_executables":"UNMEASURED: strace missing","h1_network_destinations":"UNMEASURED: strace missing"}'
if command -v strace >/dev/null 2>&1; then
  fresh_clone "$D/h1"
  set +e
  (cd "$D/h1" && unset npm_config_user_agent && strace -f -qq -e trace=execve,connect -o "$D/strace.log" -- make build >"$D/h1.log" 2>&1)
  rc=$?
  set -e
  if [[ -s "$D/strace.log" ]]; then
    h1="$(h1_parse "$D/strace.log" "$D/h1" "$(go env GOROOT)" "$(go env GOMODCACHE)" | jq -c --argjson rc "$rc" '. + {h1_make_exit:$rc}')"
  else
    h1='{"h1_host_executables":"UNMEASURED: strace produced no output (ptrace denied?)","h1_network_destinations":"UNMEASURED: strace produced no output (ptrace denied?)"}'
  fi
fi

# ---- H2 ----
fresh_clone "$D/h2"
set +e
(cd "$D/h2" && env -i PATH=/usr/bin:/bin make build >"$D/h2.log" 2>&1)
h2rc=$?
set -e
h2_ok=false; ((h2rc == 0)) && h2_ok=true

h4=0
[[ -f "$BZL_REPO_ROOT/tools/bzl/hermetic-downgrades.txt" ]] && h4="$(grep -cvE '^\s*(#|$)' "$BZL_REPO_ROOT/tools/bzl/hermetic-downgrades.txt" || true)"

result="$(jq -n --argjson h1 "$h1" --argjson ok "$h2_ok" --argjson rc "$h2rc" --arg err "$(tail -3 "$D/h2.log" | tr '\n' ' ')" \
  --arg head "$(git -C "$BZL_REPO_ROOT" rev-parse --short HEAD)" --arg d "$(date -u +%FT%TZ)" --argjson h4 "$h4" \
  '$h1 + {h2_path_scrubbed_build_ok:$ok, h2_make_exit:$rc, h2_last_lines:$err, h3:"N/A-make", h4_downgrades:$h4, head:$head, run_date:$d, system:"make"}')"
[[ -n "$OUT" ]] && printf '%s\n' "$result" > "$OUT"
printf '%s\n' "$result"
