#!/usr/bin/env bash
# Fails when the tymux Go client pinned in go.mod is not built from the same
# commit as the tymuxd release the Makefile fetches (TYMUX_VERSION), so the
# wire client and the embedded daemon cannot drift apart.
#
# Accepts a pseudo-version pin (vX.Y.Z-0.<time>-<commit12>) or a release pin
# (vX.Y.Z, resolved via the clients/go/vX.Y.Z tag). Needs network for
# `git ls-remote`; outside CI an unreachable remote only warns.
#
# Usage: tools/ci/check-tymux-pin.sh
# Env:   TYMUX_REMOTE overrides the remote (default https://github.com/tstapler/tymux)
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
remote="${TYMUX_REMOTE:-https://github.com/tstapler/tymux}"
module="github.com/tstapler/tymux/clients/go"

daemon_tag="$(sed -nE 's/^TYMUX_VERSION \?= (v[^[:space:]]+).*/\1/p' "$root/Makefile" | head -1)"
client_pin="$(awk -v m="$module" '$1 == m { print $2; exit }' "$root/go.mod")"
[[ -n "$daemon_tag" ]] || { echo "check-tymux-pin: TYMUX_VERSION not found in Makefile" >&2; exit 1; }
[[ -n "$client_pin" ]] || { echo "check-tymux-pin: $module not found in go.mod" >&2; exit 1; }

if ! refs="$(git ls-remote --tags "$remote" 2>&1)"; then
  if [[ "${CI:-}" == "true" ]]; then
    echo "check-tymux-pin: cannot reach $remote: $refs" >&2
    exit 1
  fi
  echo "check-tymux-pin: skipped, cannot reach $remote" >&2
  exit 0
fi

# Prefer the peeled commit (^{}) of an annotated tag over the tag object.
tag_commit() {
  local name="$1" peeled plain
  peeled="$(awk -v r="refs/tags/$name^{}" '$2 == r { print $1 }' <<<"$refs")"
  plain="$(awk -v r="refs/tags/$name" '$2 == r { print $1 }' <<<"$refs")"
  echo "${peeled:-$plain}"
}

daemon_commit="$(tag_commit "$daemon_tag")"
[[ -n "$daemon_commit" ]] || { echo "check-tymux-pin: Makefile TYMUX_VERSION $daemon_tag is not a tag on $remote" >&2; exit 1; }

if [[ "$client_pin" =~ -([0-9a-f]{12})$ ]]; then
  client_commit="${BASH_REMATCH[1]}"
else
  client_commit="$(tag_commit "clients/go/$client_pin")"
  [[ -n "$client_commit" ]] || { echo "check-tymux-pin: go.mod pin $client_pin has no clients/go/$client_pin tag on $remote" >&2; exit 1; }
fi

if [[ "$daemon_commit" != "$client_commit"* && "$client_commit" != "$daemon_commit"* ]]; then
  echo "check-tymux-pin: Makefile TYMUX_VERSION $daemon_tag is commit ${daemon_commit:0:12}, but go.mod pins the client at ${client_commit:0:12} ($client_pin)." >&2
  echo "Bump both together: Makefile TYMUX_VERSION and the $module require in go.mod." >&2
  exit 1
fi
echo "tymux pin consistent: daemon $daemon_tag and client $client_pin are commit ${daemon_commit:0:12}"
