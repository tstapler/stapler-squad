#!/usr/bin/env bash
# Owner-run: install the daily orphan Bazel output-base timer as a user systemd unit.
#   install-bazel-timers.sh [--dry-run | --uninstall]
# --dry-run prints every command and changes nothing. Agents never run this without --dry-run.
set -eu
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$here/../.." && pwd)
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
link="$HOME/.local/share/stapler-squad-bazel/repo"
mode=install
[ "$(git -C "$repo" rev-parse --git-dir)" = "$(git -C "$repo" rev-parse --git-common-dir)" ] \
  || echo "WARNING: $repo is a linked worktree; the unit would break when it is removed. Run this from the main checkout." >&2
case "${1:-}" in
  "") ;;
  --dry-run) mode=dry ;;
  --uninstall) mode=uninstall ;;
  *) echo "usage: $0 [--dry-run | --uninstall]" >&2; exit 2 ;;
esac

run() { if [ "$mode" = dry ]; then echo "+ $*"; else "$@"; fi; }

if [ "$mode" = uninstall ]; then
  systemctl --user disable --now bazel-orphan-outputbases.timer || true
  rm -f "$unit_dir/bazel-orphan-outputbases.service" "$unit_dir/bazel-orphan-outputbases.timer" "$link"
  systemctl --user daemon-reload
  exit 0
fi

run mkdir -p "$unit_dir" "$(dirname "$link")"
run ln -sfn "$repo" "$link"
run cp "$here/bazel-orphan-outputbases.service" "$here/bazel-orphan-outputbases.timer" "$unit_dir/"
run systemctl --user daemon-reload
run systemctl --user enable --now bazel-orphan-outputbases.timer
[ "$mode" = dry ] || systemctl --user is-enabled bazel-orphan-outputbases.timer
