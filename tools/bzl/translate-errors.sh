#!/usr/bin/env bash
# Translate a captured Bazel (or plain go) failure into one remedy line from errors.yaml.
#   translate-errors.sh [--source bazel|go] [--exit N] [--hints-only] < log   passes the log through, then appends the hint
#   translate-errors.sh --text E7|E7.repeat|E7.timeout                        prints a wrapper-emitted message template
# Rows are tried in priority order; the first match wins, so at most one hint is printed.
# Exit status is always 0: this is a filter. bash 3.2 compatible.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
table="${ERRORS_YAML:-$here/errors.yaml}"
source_kind=bazel
rc=""
hints_only=""
text=""
while [ $# -gt 0 ]; do
  case "$1" in
    --source) source_kind="${2:?--source needs bazel|go}"; shift 2 ;;
    --exit) rc="${2:?--exit needs a status}"; shift 2 ;;
    --hints-only) hints_only=1; shift ;;
    --text) text="${2:?--text needs a row id}"; shift 2 ;;
    *) echo "translate-errors.sh: unknown argument $1" >&2; exit 2 ;;
  esac
done

# Shared yaml reader: prints "<id>\t<key>\t<value>" for every key of every row.
read_table() {
  awk '
    /^- id:/ { id=$3; next }
    /^  [a-z_]+:/ && id != "" {
      k=$1; sub(/:$/,"",k)
      v=$0; sub(/^  [a-z_]+:[ ]*/,"",v)
      if ((v ~ /^".*"$/) || (v ~ /^'"'"'.*'"'"'$/)) v=substr(v,2,length(v)-2)
      printf "%s\t%s\t%s\n", id, k, v
    }' "$table"
}

if [ -n "$text" ]; then
  id=${text%%.*}; key=hint
  case "$text" in *.repeat) key=hint_repeat ;; *.timeout) key=hint_timeout ;; esac
  read_table | awk -F'\t' -v id="$id" -v key="$key" '$1==id && $2==key { print $3 }'
  exit 0
fi

input=$(mktemp "${TMPDIR:-/tmp}/translate-errors.XXXXXX")
trap 'rm -f "$input"' EXIT
cat > "$input"
[ -n "$hints_only" ] || cat "$input"

read_table | awk -F'\t' -v src="$source_kind" -v rc="$rc" -v root="$root" -v input="$input" '
  { id=$1; k=$2; v=$3
    if (!(id in seen)) { seen[id]=1; order[++n]=id }
    if (k=="when") { nw[id]++; w[id, nw[id]]=v }
    else if (k=="when_exit") { we[id]=v }
    else attr[id, k]=v }
  function matches(line, spec,    parts, np, a, b, i) {
    np = split(spec, parts, / && /)
    a = parts[1]; b = (np > 1) ? parts[2] : ""
    return (line ~ a) && (b == "" || line ~ b)
  }
  function row_matches(id,    i, line, ne, ep, hit) {
    hit = 0
    while ((getline line < input) > 0) {
      for (i = 1; i <= nw[id]; i++) if (matches(line, w[id, i])) hit = 1
      if (id in we) {
        ne = split(we[id], ep, / && /)
        if (rc == ep[1] && line ~ ep[2]) hit = 1
      }
      if (hit) break
    }
    close(input)
    return hit
  }
  END {
    for (p = 1; p <= 98; p++) for (j = 1; j <= n; j++) {
      id = order[j]
      if (attr[id, "priority"] != p || attr[id, "source"] != src) continue
      if (rc == "0" && attr[id, "on_success"] != "true") continue
      if (!row_matches(id)) continue
      h = attr[id, "hint"]
      nf = attr[id, "needs_file"]
      if (nf != "" && system("test -e \"" root "/" nf "\"") != 0) h = attr[id, "hint_fallback"]
      print h
      if (attr[id, "doc"] != "") print "See docs/how-to/troubleshoot-bazel.md#" attr[id, "doc"]
      exit
    }
  }'
exit 0
