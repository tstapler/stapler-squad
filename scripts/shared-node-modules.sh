#!/usr/bin/env bash
# Materialize web-app/node_modules as a symlink into a content-addressed
# shared directory instead of a full per-worktree pnpm install.
#
# Why: pnpm's global content-addressable store (~/.local/share/pnpm/store)
# already dedupes package *bytes* across every worktree of this repo -- that
# part was never the problem. What isn't deduped is the *file/symlink tree
# structure* pnpm materializes into each worktree's node_modules (~82,000
# filesystem entries per install, measured via `find web-app/node_modules |
# wc -l`), and that's what was driving btrfs metadata-chunk/inode pressure
# across the dozens of concurrent git worktrees this repo accumulates. See
# docs/explanation/package-management-evaluation.md for the measurements
# and the alternatives (Yarn PnP, Bazel, Bun) this was weighed against.
#
# The shared directory is keyed by a hash of pnpm-lock.yaml PLUS the content
# of any local `file:`-protocol dependency (e.g. web-app's in-repo
# eslint-plugin-analytics / eslint-plugin-rpc-lifecycle). pnpm's lockfile
# records those as `{directory: ..., type: directory}` with no content
# digest, so keying on the lockfile alone would let a worktree that edited
# one of those plugins silently keep serving another worktree's stale copy
# through the shared tree -- folding their file content into the key
# closes that gap (verified empirically: editing a rule file with the
# lockfile untouched does change the computed hash).
#
# First worktree to need a given hash does a real `pnpm install` into the
# shared directory (guarded by flock so two worktrees racing on a brand-new
# hash don't corrupt it -- verified empirically with two concurrent runs).
# Every other worktree with the same hash just symlinks node_modules at it,
# skipping the install entirely.
#
# Usage: scripts/shared-node-modules.sh <web-app-dir>
# Opt out (always do a plain local install, e.g. to debug a suspected
# shared-tree issue) with:
#   STAPLER_SQUAD_SHARED_NODE_MODULES=0 scripts/shared-node-modules.sh web-app
set -euo pipefail

WEBAPP_DIR="${1:?usage: shared-node-modules.sh <web-app-dir>}"
SHARED_ROOT="${STAPLER_SQUAD_SHARED_NODE_MODULES_DIR:-$HOME/.stapler-squad/shared-node-modules}"

cd "$WEBAPP_DIR"

if [ "${STAPLER_SQUAD_SHARED_NODE_MODULES:-1}" = "0" ]; then
	echo "shared-node-modules: STAPLER_SQUAD_SHARED_NODE_MODULES=0, doing a plain local pnpm install"
	if [ -L node_modules ]; then
		rm node_modules
	fi
	exec pnpm install --frozen-lockfile
fi

if ! command -v jq >/dev/null 2>&1; then
	echo "shared-node-modules: jq not found, falling back to a plain local pnpm install" >&2
	exec pnpm install --frozen-lockfile
fi

# Local file:-protocol deps declared in package.json (dependencies,
# devDependencies, optionalDependencies), with the "file:" prefix stripped
# to a plain relative path. Generic over package.json contents rather than
# hardcoding today's two plugin names, so a future local dep is covered
# automatically.
local_file_dep_dirs() {
	jq -r '[(.dependencies // {}), (.devDependencies // {}), (.optionalDependencies // {})]
		| add | to_entries[] | select(.value | startswith("file:"))
		| .value | sub("^file:"; "")' package.json
}

compute_hash() {
	sha256sum pnpm-lock.yaml
	local_file_dep_dirs | while IFS= read -r d; do
		[ -d "$d" ] || { echo "shared-node-modules: local file: dependency dir missing: $d" >&2; exit 1; }
		find "$d" -type f -print0 | sort -z | xargs -0 sha256sum
	done
}

HASH="$(compute_hash | sha256sum | cut -d' ' -f1)"
SHARED="$SHARED_ROOT/$HASH"

# Fast path: already linked at the right place, nothing to do.
if [ -L node_modules ] && [ "$(readlink node_modules)" = "$SHARED/node_modules" ] && [ -f "$SHARED/.install-complete" ]; then
	exit 0
fi

mkdir -p "$SHARED_ROOT"
exec 200>"$SHARED_ROOT/$HASH.lock"
flock 200

if [ ! -f "$SHARED/.install-complete" ]; then
	echo "shared-node-modules: no shared install for $HASH yet, installing into $SHARED"
	rm -rf "$SHARED"
	mkdir -p "$SHARED"
	cp package.json pnpm-lock.yaml "$SHARED/"
	local_file_dep_dirs | while IFS= read -r d; do
		mkdir -p "$SHARED/$(dirname "$d")"
		rm -rf "${SHARED:?}/${d:?}"
		cp -r "$d" "$SHARED/$d"
	done
	( cd "$SHARED" && pnpm install --frozen-lockfile )
	touch "$SHARED/.install-complete"
else
	echo "shared-node-modules: reusing existing shared install at $SHARED"
fi
flock -u 200

if [ -L node_modules ]; then
	rm node_modules
elif [ -e node_modules ]; then
	echo "shared-node-modules: node_modules exists and isn't a symlink we manage; leaving it alone (rm -rf web-app/node_modules first to opt in)" >&2
	exit 0
fi

ln -s "$SHARED/node_modules" node_modules
echo "shared-node-modules: linked node_modules -> $SHARED/node_modules"
