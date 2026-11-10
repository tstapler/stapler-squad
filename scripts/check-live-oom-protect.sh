#!/usr/bin/env bash
# Exit 0 when the live stapler-squad unit is protected from the OOM killer (configured
# OOMScoreAdjust <= -500 AND the running main process actually has that score); exit 3 with
# "OWNER ACTION" otherwise. Read-only. SSQ_UNIT overrides the unit name.
# See docs/how-to/protect-live-service-from-bazel.md.
set -u
unit="${SSQ_UNIT:-stapler-squad.service}"
threshold=-500

configured=$(systemctl --user show "$unit" -p OOMScoreAdjust --value 2>/dev/null || true)
pid=$(systemctl --user show "$unit" -p MainPID --value 2>/dev/null || true)
effective=""
if [ -n "$pid" ] && [ "$pid" != 0 ] && [ -r "/proc/$pid/oom_score_adj" ]; then
  effective=$(cat "/proc/$pid/oom_score_adj")
fi

echo "unit=$unit configured_OOMScoreAdjust=${configured:-unknown} main_pid=${pid:-none} effective_oom_score_adj=${effective:-unknown}"

if [ -n "$configured" ] && [ "$configured" -le "$threshold" ] 2>/dev/null \
   && [ -n "$effective" ] && [ "$effective" -le "$threshold" ] 2>/dev/null; then
  exit 0
fi
echo "OWNER ACTION: $unit is not OOM-protected (need configured and effective <= $threshold). See docs/how-to/protect-live-service-from-bazel.md" >&2
exit 3
