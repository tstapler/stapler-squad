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

# --- Local (offline) checks of the Bazel pins; skipped only when MODULE.bazel is absent. ---
module_bazel="$root/MODULE.bazel"
if [[ -f "$module_bazel" ]]; then
  checksums="$root/scripts/tymuxd-checksums.txt"
  pin_file="$root/tools/bzl/tmux/PIN"
  fail=0
  complain() { echo "check-tymux-pin: $*" >&2; fail=1; }

  # "<repo-name> <sha256> <first url>" for every http_archive block in MODULE.bazel.
  archives="$(awk '
    /^http_archive\(/ { inb = 1; name = ""; sha = ""; url = ""; next }
    inb && /^ +name = "/ { split($0, a, "\""); name = a[2] }
    inb && /sha256 = "/ { split($0, a, "\""); sha = a[2] }
    inb && /https?:\/\// && url == "" { match($0, /https?:\/\/[^"]+/); url = substr($0, RSTART, RLENGTH) }
    inb && /^\)/ { print name, sha, url; inb = 0 }
  ' "$module_bazel")"

  count=0
  while read -r name sha url; do
    [[ "$name" == tymuxd_* ]] || continue
    count=$((count + 1))
    target="${name#tymuxd_}"; target="${target//_/-}"; target="${target/x86-64/x86_64}"
    want="$(awk -v v="$daemon_tag" -v t="$target" '$1 == v && $2 == t { print $3 }' "$checksums")"
    if [[ -z "$want" ]]; then
      complain "$checksums has no '$daemon_tag $target' line for MODULE.bazel repo $name"
    elif [[ "$sha" != "$want" ]]; then
      complain "MODULE.bazel $name sha256 $sha differs from $checksums ($want)"
    fi
    [[ "$url" == *"/download/$daemon_tag/"* ]] || complain "MODULE.bazel $name url is not release $daemon_tag: $url"
  done <<<"$archives"
  [[ "$count" -gt 0 ]] || complain "no tymuxd_* http_archive found in MODULE.bazel"

  pin_tag="$(sed -n 's/^tag=//p' "$pin_file")"
  pin_sha="$(sed -n 's/^sha256=//p' "$pin_file")"
  pin_commit="$(sed -n 's/^commit=//p' "$pin_file")"
  tmux_line="$(awk '$1 == "tmux_src"' <<<"$archives")"
  read -r _ tmux_sha tmux_url <<<"$tmux_line"
  [[ -n "$pin_tag" && -n "$pin_sha" && -n "$pin_commit" ]] || complain "$pin_file needs tag, sha256 and commit"
  [[ "$tmux_sha" == "$pin_sha" ]] || complain "MODULE.bazel tmux_src sha256 ${tmux_sha:-<missing>} differs from $pin_file ($pin_sha)"
  [[ "$tmux_url" == *"/download/$pin_tag/tmux-$pin_tag.tar.gz" ]] || complain "MODULE.bazel tmux_src url ${tmux_url:-<missing>} is not tag $pin_tag of $pin_file"

  gitlink="$(git -C "$root" ls-tree HEAD third_party/tmux 2>/dev/null | awk '$1 == "160000" { print $3 }')"
  if [[ -z "$gitlink" ]]; then
    complain "third_party/tmux is not a gitlink at HEAD"
  elif [[ "$gitlink" != "$pin_commit" ]]; then
    complain "third_party/tmux gitlink $gitlink differs from $pin_file commit $pin_commit"
  fi
  [[ "$fail" -eq 0 ]] || exit 1
fi

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
