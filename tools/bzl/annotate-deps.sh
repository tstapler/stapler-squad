#!/usr/bin/env bash
# Summarise external-dependency changes between two revisions so they cannot hide in a collapsed lock diff
# (Task 11.6c). Reads go.mod, MODULE.bazel and MODULE.bazel.lock (the lock only decides "no dependency change").
#   annotate-deps.sh <base> <head>
# Per category prints "added", "removed" and "upgraded (old -> new)" for Go modules (require lines), bazel_dep
# lines and use_repo names, or "no dependency change".
set -eu
base="${1:?usage: $0 <base> <head>}"; head="${2:?usage: $0 <base> <head>}"
T=$(mktemp -d "${TMPDIR:-/tmp}/annotate-deps.XXXXXX"); trap 'rm -rf "$T"' EXIT

show() { git show "$1:$2" 2>/dev/null || true; }

gomods() { # "<module> <version>" per require line
  awk '
    /^require \(/ { inreq=1; next }
    inreq && /^\)/ { inreq=0; next }
    inreq { sub(/\/\/.*/, ""); if (NF >= 2) print $1, $2; next }
    /^require / { sub(/\/\/.*/, ""); print $2, $3 }'
}
bazeldeps() { # "<name> <version>" per bazel_dep call
  tr '\n' ' ' | grep -oE 'bazel_dep\([^)]*\)' | awk '{
    n=""; v=""
    if (match($0, /name *= *"[^"]*"/)) { n=substr($0, RSTART, RLENGTH); sub(/^[^"]*"/, "", n); sub(/"$/, "", n) }
    if (match($0, /version *= *"[^"]*"/)) { v=substr($0, RSTART, RLENGTH); sub(/^[^"]*"/, "", v); sub(/"$/, "", v) }
    print n, (v == "" ? "-" : v) }'
}
userepos() { # "<repo> -" per name inside use_repo(...) calls (first argument is the extension proxy)
  tr '\n' ' ' | grep -oE 'use_repo\([^)]*\)' | sed -E 's/^use_repo\(//; s/\)$//' \
    | awk -F, '{ for (i = 2; i <= NF; i++) { s=$i; gsub(/^[ ]+|[ ]+$/, "", s); gsub(/"/, "", s); if (s != "") print s, "-" } }'
}

diff_cat() { # <label> <base-list> <head-list>
  local label=$1 b=$2 h=$3 out
  out=$(awk -v label="$label" '
    FILENAME == ARGV[1] { old[$1]=$2; next }
    { new[$1]=$2 }
    END {
      for (k in new) { if (!(k in old)) add[k]=new[k]; else if (old[k] != new[k]) up[k]=old[k] " -> " new[k] }
      for (k in old) if (!(k in new)) rem[k]=old[k]
      for (k in add) printf "%s added: %s %s\n", label, k, (add[k] == "-" ? "" : add[k])
      for (k in rem) printf "%s removed: %s %s\n", label, k, (rem[k] == "-" ? "" : rem[k])
      for (k in up) printf "%s upgraded: %s (%s)\n", label, k, up[k]
    }' "$b" "$h" | sed 's/ $//' | sort)
  [ -z "$out" ] || printf '%s\n' "$out"
}

for rev in base head; do
  r=$base; [ "$rev" = head ] && r=$head
  show "$r" go.mod | gomods | sort > "$T/go.$rev"
  show "$r" MODULE.bazel | bazeldeps | sort > "$T/dep.$rev"
  show "$r" MODULE.bazel | userepos | sort -u > "$T/use.$rev"
done

result=$( { diff_cat "go module" "$T/go.base" "$T/go.head"
            diff_cat "bazel_dep" "$T/dep.base" "$T/dep.head"
            diff_cat "use_repo" "$T/use.base" "$T/use.head"; } )
if [ -z "$result" ]; then echo "no dependency change"; else printf '%s\n' "$result"; fi
