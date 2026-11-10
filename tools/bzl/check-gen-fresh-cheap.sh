#!/usr/bin/env bash
# Cheap GenFreshness (Task 2.4d): needs no Bazel. `make sync` writes gen/.sync-inputs.sha256 (the hash of
# the generator inputs); this recomputes it and compares.
#   check-gen-fresh-cheap.sh          print "gen: fresh" (exit 0) or "gen: STALE - run 'make sync'" (exit 1)
#   check-gen-fresh-cheap.sh --warn   same output, but always exit 0
#   check-gen-fresh-cheap.sh --print  print the current inputs hash (used by sync to write the stamp)
#   check-gen-fresh-cheap.sh --write  write the stamp
#   check-gen-fresh-cheap.sh --note   Bazel-run wrappers: print a one-line note when stale, silent when fresh, exit 0
#   check-gen-fresh-cheap.sh --strict native-Go wrappers: exit 1 when a stamp exists and no longer matches; with no
#                                     stamp (fresh clone, CI) print a note and exit 0, because make regenerates anyway
set -u
root=$(git rev-parse --show-toplevel 2>/dev/null) || root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root" || exit 2
stamp=gen/.sync-inputs.sha256

inputs_hash() {
  { find proto -name '*.proto' -type f 2>/dev/null
    find . -maxdepth 1 -name 'buf*.yaml' -type f 2>/dev/null | sed 's#^\./##'
    find session/ent/schema -name '*.go' -type f 2>/dev/null
    [ -f session/ent/generate.go ] && echo session/ent/generate.go
  } | LC_ALL=C sort | xargs sha256sum | sha256sum | cut -d' ' -f1
}

case "${1:-}" in
  --print) inputs_hash; exit 0 ;;
  --write) mkdir -p gen && inputs_hash > "$stamp"; exit 0 ;;
esac
case "${1:-}" in
  --note)
    if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$(inputs_hash)" ]; then exit 0; fi
    echo "note: gen/ is stale for plain go and gopls; Bazel is unaffected. Run 'make sync'."
    exit 0 ;;
  --strict)
    [ -f "$stamp" ] || { echo "note: gen/ has not been synced here; make regenerates it for this target"; exit 0; }
    if [ "$(cat "$stamp")" = "$(inputs_hash)" ]; then exit 0; fi
    echo "gen: STALE - run 'make sync'"
    exit 1 ;;
esac
warn=0; [ "${1:-}" = --warn ] && warn=1
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$(inputs_hash)" ]; then
  echo "gen: fresh"; exit 0
fi
echo "gen: STALE - run 'make sync'"
[ "$warn" = 1 ] && exit 0
exit 1
