#!/bin/bash
# Interleaved baseline/workload runs of the EDR experiment (docs/explanation/edr-subprocess-contention.md).
# Build first: (cd scripts/edr-experiment/load && go build -o /tmp/edr-exp/load .) and
# cc -O2 -o /tmp/edr-exp/pspawn scripts/edr-experiment/pspawn.c. F is the Falcon pid, SPID syspolicyd's:
# find them with `ps -axo pid,comm | grep -E 'falcon.Agent|syspolicyd'`. No root needed.
export SPID=474
STATROOT=/Users/tstapler/code/github.com/tstapler/stapler-squad/web-app/node_modules
BIGBIN=/Users/tstapler/code/github.com/tstapler/stapler-squad/stapler-squad
export STATROOT BIGBIN
L=/tmp/edr-exp/load; F=575; D=8
W=(pwrite:2000 pwrite:8000 true:200 sh:100 zshl:20 py:40 shebang_cached:100 shebang_fresh_same:15 shebang_rewrite:15 true_copy_fresh:15 writeonly_fresh:15 createwrite:3000 openread:5000 stat:0 tcp:0)
for pair in 1 2 3 4 5; do
  list=("${W[@]}")
  if [ $((pair % 2)) = 0 ]; then
    list=()
    for ((i = ${#W[@]} - 1; i >= 0; i--)); do list+=("${W[i]}"); done
  fi
  for w in "${list[@]}"; do
    m=${w%%:*}; r=${w##*:}
    "$L" sleep "$D" 0 "$F" | sed "s/^/P$pair BASE /"
    "$L" "$m" "$D" "$r" "$F" | sed "s/^/P$pair /"
  done
done
echo DONE
