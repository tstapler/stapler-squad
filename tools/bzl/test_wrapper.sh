#!/usr/bin/env bash
# Bazel `--run_under` wrapper for every test (.bazelrc `test --run_under`). Builds, per test
# process, the private environment the Epic 0 census (S8/S9) showed tests need:
#   HOME under TEST_TMPDIR with a .gitconfig identity (else every git-committing test fails),
#   a private short TMUX_TMPDIR so tmux never reaches the default server socket,
#   tmux/gh/go on PATH (Bazel's test PATH is /usr/bin:/bin),
#   a UTF-8 LANG (tmux sanitises tabs in -F output otherwise).
# It sets no STAPLER_SQUAD_TMUX_SOCKET: tests that assert their own socket names break under a
# global one. TMPDIR is pinned to /tmp (short: unix-socket paths stay under 108 bytes) because a Bazel server started with TMPDIR
# under $HOME makes unsandboxed tests inherit it (session/vc then walks up to ~/.jj, unix-socket
# paths overflow).
# Knobs: SSQ_TEST_EXTRA_PATH (colon list searched for host tools, via --test_env),
#        SSQ_TEST_HOST_TOOLS (space list, default "tmux gh go").
set -u

tmp="${TEST_TMPDIR:-}"
if [ -z "$tmp" ]; then
  echo "test_wrapper: TEST_TMPDIR is unset; this wrapper only runs under 'bazel test'" >&2
  exit 2
fi

home="$tmp/home"
mkdir -p "$home"
cat > "$home/.gitconfig" <<'GITCFG'
[user]
	name = Test User
	email = test@example.invalid
[init]
	defaultBranch = main
GITCFG
export HOME="$home"

# `mktemp -d /tmp/...` is the sandbox's private /tmp (hermetic tmp) or the host /tmp when a
# target opts out of sandboxing; a unique dir per process either way.
tmux_dir=$(mktemp -d /tmp/ssq-tmux.XXXXXX) || exit 2
export TMUX_TMPDIR="$tmux_dir"
unset TMUX TMUX_PANE
export TMPDIR=/tmp
# Bazel's clean env has no locale; tmux then prints "_" for the tab in `-F` formats (session/mux
# ScanByUserOptions splits on it), so give it a UTF-8 locale.
export LANG="${LANG:-C.UTF-8}"

bin="$tmp/hostbin"
mkdir -p "$bin"
search="${SSQ_TEST_EXTRA_PATH:+$SSQ_TEST_EXTRA_PATH:}/home/linuxbrew/.linuxbrew/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
for tool in ${SSQ_TEST_HOST_TOOLS:-tmux gh go}; do
  found=$(PATH="$search" command -v "$tool" 2>/dev/null || true)
  if [ -n "$found" ]; then ln -sf "$found" "$bin/$tool"; fi
done
export PATH="$bin:$PATH"

# shellcheck disable=SC2329
cleanup() {
  # TMUX_TMPDIR is private (set above), so this reaches only this process's tmux servers.
  # shellcheck disable=SC2317  # runs from the EXIT trap
  case "$tmux_dir" in /tmp/ssq-tmux.*) tmux kill-server >/dev/null 2>&1 || true; rm -rf "$tmux_dir" ;; esac
}
child=""
trap 'if [ -n "$child" ]; then kill -TERM "$child" 2>/dev/null; fi' TERM INT
trap cleanup EXIT

"$@" &
child=$!
status=0
wait "$child" || status=$?
# a trapped signal interrupts the first wait; reap the child for its real status
if kill -0 "$child" 2>/dev/null; then wait "$child" || status=$?; fi
exit "$status"
