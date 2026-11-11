#!/bin/sh
# shellcheck shell=sh
#
# install-service-preflight.test.sh — regression test for the Story 8.5
# (project_plans/web-app-bun-build) `install-service-preflight` Makefile
# target: `make install-service` must refuse to build/restart when the
# deployed unit lacks --tmux-keep-server, since that restart would destroy
# every live tmux session (docs/explanation/tmux-keep-server-on-restart.md).
#
# The guard is written as Make `$(shell ...)`/`$(error ...)` expansions
# rather than a shell conditional inside the recipe body, specifically so it
# fires during `make -n` (dry-run) expansion too — Make expands recipe text
# (including $(error)) before printing it, even under -n, it just skips
# *executing* the printed command. That lets this test exercise the real
# guard logic without ever letting `install-service` build or touch the
# live service.
#
# Three cases (Task 8.5d's AC):
#   1. flag present                              -> make -n exits 0
#   2. flag absent, no ACCEPT_SESSION_LOSS        -> make -n exits non-zero,
#                                                     naming the doc
#   3. flag absent, ACCEPT_SESSION_LOSS=1         -> make -n exits 0
#
# Usage: sh scripts/install-service-preflight.test.sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

NO_FLAG_UNIT="$TMP_DIR/fake-no-flag.service"
WITH_FLAG_UNIT="$TMP_DIR/fake-with-flag.service"

cat > "$NO_FLAG_UNIT" <<'EOF'
[Service]
ExecStart=/opt/stapler-squad/stapler-squad --remote-access
EOF

cat > "$WITH_FLAG_UNIT" <<'EOF'
[Service]
ExecStart=/opt/stapler-squad/stapler-squad --remote-access --tmux-keep-server
EOF

ok=1

run_preflight() {
    # $1=unit file  $2...=extra make vars
    unit="$1"
    shift
    (cd "$REPO_ROOT" && make -n install-service-preflight UNAME_S=Linux \
        "INSTALL_SERVICE_LINUX_UNIT=$unit" "$@")
}

echo "=== case 1: --tmux-keep-server present ==="
if out="$(run_preflight "$WITH_FLAG_UNIT" 2>&1)"; then
    echo "PASS: exits 0 when the flag is present"
else
    echo "FAIL: expected exit 0 when the flag is present; got:" >&2
    echo "$out" >&2
    ok=0
fi

echo "=== case 2: --tmux-keep-server absent, no override ==="
if out="$(run_preflight "$NO_FLAG_UNIT" 2>&1)"; then
    echo "FAIL: expected non-zero exit when the flag is absent and ACCEPT_SESSION_LOSS is unset; got exit 0:" >&2
    echo "$out" >&2
    ok=0
else
    if echo "$out" | grep -q 'docs/explanation/tmux-keep-server-on-restart.md'; then
        echo "PASS: exits non-zero and names docs/explanation/tmux-keep-server-on-restart.md"
    else
        echo "FAIL: exited non-zero but did not name docs/explanation/tmux-keep-server-on-restart.md; got:" >&2
        echo "$out" >&2
        ok=0
    fi
fi

echo "=== case 3: --tmux-keep-server absent, ACCEPT_SESSION_LOSS=1 ==="
if out="$(run_preflight "$NO_FLAG_UNIT" "ACCEPT_SESSION_LOSS=1" 2>&1)"; then
    echo "PASS: exits 0 when ACCEPT_SESSION_LOSS=1 overrides the missing flag"
else
    echo "FAIL: expected exit 0 with ACCEPT_SESSION_LOSS=1; got:" >&2
    echo "$out" >&2
    ok=0
fi

echo "=== sanity: unrelated targets (lint) are not gated by this guard ==="
if out="$(cd "$REPO_ROOT" && make -n lint "INSTALL_SERVICE_LINUX_UNIT=$NO_FLAG_UNIT" UNAME_S=Linux 2>&1)"; then
    echo "PASS: make -n lint is unaffected by a missing --tmux-keep-server"
else
    echo "FAIL: make -n lint unexpectedly failed:" >&2
    echo "$out" >&2
    ok=0
fi

echo "=== case 4: make BUILD_SYSTEM=bazel -n install-service builds, copies to ./stapler-squad, then installs ==="
bz_out="$(cd "$REPO_ROOT" && make BUILD_SYSTEM=bazel -n install-service UNAME_S=Linux \
    "INSTALL_SERVICE_LINUX_UNIT=$WITH_FLAG_UNIT" 2>&1)" || { echo "FAIL: make -n failed:" >&2; echo "$bz_out" >&2; ok=0; }
for pat in 'bazel build --config=release //:stapler-squad' 'cp .*\./stapler-squad' 'scripts/install-service.sh' 'SSQ_VERIFY_BIN_TOOL='; do
    if printf '%s\n' "$bz_out" | grep -qE "$pat"; then
        echo "PASS: recipe contains: $pat"
    else
        echo "FAIL: recipe is missing: $pat" >&2
        ok=0
    fi
done
if printf '%s\n' "$bz_out" | grep -qE 'ExecStart.*/bazel-(bin|out)/|STAPLER_SQUAD_BIN="[^"]*/bazel-(bin|out)/'; then
    echo "FAIL: ExecStart/STAPLER_SQUAD_BIN points into bazel-*" >&2
    ok=0
fi
build_line="$(printf '%s\n' "$bz_out" | grep -nE 'bazel build --config=release //:stapler-squad' | head -1 | cut -d: -f1)"
cp_line="$(printf '%s\n' "$bz_out" | grep -nE 'cp bazel-bin/stapler-squad \./stapler-squad' | head -1 | cut -d: -f1)"
inst_line="$(printf '%s\n' "$bz_out" | grep -nE 'scripts/install-service.sh' | head -1 | cut -d: -f1)"
if [ -n "$build_line" ] && [ -n "$cp_line" ] && [ -n "$inst_line" ] && [ "$build_line" -lt "$cp_line" ] && [ "$cp_line" -lt "$inst_line" ]; then
    echo "PASS: order is build, cp, install-service.sh"
else
    echo "FAIL: expected build < cp < install-service.sh (got $build_line, $cp_line, $inst_line)" >&2
    ok=0
fi

echo "=== case 5: BUILD_SYSTEM=make install-service recipe is unchanged (no bazel, no verify tool) ==="
mk_out="$(cd "$REPO_ROOT" && make BUILD_SYSTEM=make -n install-service UNAME_S=Linux \
    "INSTALL_SERVICE_LINUX_UNIT=$WITH_FLAG_UNIT" 2>&1)" || { echo "FAIL: make -n failed:" >&2; echo "$mk_out" >&2; ok=0; }
if printf '%s\n' "$mk_out" | grep -qE 'bazel build|tools/bazel|SSQ_VERIFY|\[build-system=bazel\]'; then
    echo "FAIL: the make recipe mentions bazel or SSQ_VERIFY:" >&2
    printf '%s\n' "$mk_out" | grep -E 'bazel build|tools/bazel|SSQ_VERIFY|\[build-system=bazel\]' >&2
    ok=0
else
    echo "PASS: the make recipe has no bazel or SSQ_VERIFY text"
fi

echo "=== case 6: --dry-run unit text carries the OOM settings and --tmux-keep-server ==="
if [ "$(uname -s)" = "Linux" ]; then
    printf '#!/bin/sh\necho "stapler-squad version 0.0.0-test"\n' > "$TMP_DIR/fake-ssq"
    chmod +x "$TMP_DIR/fake-ssq"
    mkdir -p "$TMP_DIR/home"
    unit_text="$(HOME="$TMP_DIR/home" STAPLER_SQUAD_BIN="$TMP_DIR/fake-ssq" SERVICE_ENV_FILE="$TMP_DIR/none.env" \
        sh "$SCRIPT_DIR/install-service.sh" --dry-run 2>&1)" || { echo "FAIL: --dry-run failed:" >&2; echo "$unit_text" >&2; ok=0; }
    for pat in '^OOMScoreAdjust=-900$' '^OOMPolicy=continue$' '^ExecStart=.*--tmux-keep-server'; do
        if printf '%s\n' "$unit_text" | grep -qE "$pat"; then
            echo "PASS: unit contains $pat"
        else
            echo "FAIL: unit is missing $pat" >&2
            ok=0
        fi
    done
    if [ -n "$(ls -A "$TMP_DIR/home")" ]; then
        echo "FAIL: --dry-run wrote into HOME" >&2
        ok=0
    fi
else
    echo "SKIP: --dry-run is Linux-only"
fi

if [ "$ok" = "0" ]; then
    echo "install-service-preflight.test.sh: FAILED" >&2
    exit 1
fi

echo "install-service-preflight.test.sh: all cases PASSED"
