#!/usr/bin/env bash
# A fake AskUserQuestion pane for the structured-question e2e (session:guidance-structured).
# Draws the captured Claude Code dialog, records the ONE key it receives into $1, then redraws
# the idle composer, like the agent moving on. Dialog and composer are the unit-test fixtures.
# With "sticky" as $2 the first key leaves the dialog on screen (the agent ignored it) and only a second
# key closes it; $1 then holds every key received, in order.
# With "tabbed" as $2 it replays the measured two-question dialog (Spike S1c): $1 then holds the name of every
# key received (Enter or Down), one per line, and the pane advances one frame per key through the 9-key answer
# for Redis + [A, B]. "tabbed-drift" swaps in an unrelated dialog after the third key, so the sequence aborts partway.
set -u
root="$(cd "$(dirname "$0")/../../.." && pwd)"
out="$1"
dialogs="$root/session/testdata/reply_dialogs"
dialog="$dialogs/question_single_select_3opts.txt"
: > "$out"

# tabbed_frame N prints the pane after N keys of the recorded sequence. Frames 2-3 and 5-6 are the real
# AB_toggled capture with only its focus and checkbox marks moved, as in server/services/pane_reply_sequence_test.go.
tabbed_frame() {
  local ab="$dialogs/claude_auq_twoq_step2_multiselect_AB_toggled.txt"
  case "$1" in
    0) cat "$dialogs/claude_auq_twoq_step1_single_select.txt" ;;
    1) cat "$dialogs/claude_auq_twoq_step2_multiselect_initial.txt" ;;
    2) sed -e 's/❯ 2\. \[✔\] B/  2. [ ] B/' -e 's/  1\. \[✔\] A/❯ 1. [✔] A/' "$ab" ;;
    3) sed -e 's/❯ 2\. \[✔\] B/❯ 2. [ ] B/' "$ab" ;;
    4) cat "$ab" ;;
    5) sed -e 's/❯ 2\. \[✔\] B/  2. [✔] B/' -e 's/  3\. \[ \] C/❯ 3. [ ] C/' "$ab" ;;
    6) sed -e 's/❯ 2\. \[✔\] B/  2. [✔] B/' -e 's/  4\. \[ \] Type something/❯ 4. [ ] Type something/' "$ab" ;;
    7) cat "$dialogs/claude_auq_twoq_step2_multiselect_submit_focused.txt" ;;
    8) cat "$dialogs/claude_auq_twoq_review_submit.txt" ;;
    *) cat "$root/session/testdata/dialog_presence/idle_composer.txt" ;;
  esac
}

if [ "${2:-}" = tabbed ] || [ "${2:-}" = tabbed-drift ]; then
  n=0
  tabbed_frame 0
  while [ "$n" -lt 9 ]; do
    IFS= read -r -n1 -s key
    if [ "$key" = $'\e' ]; then
      read -r -n2 -s -t 1 rest
      key="$key$rest"
    fi
    case "$key" in
      "" | $'\r') echo Enter >> "$out" ;;
      $'\e[B') echo Down >> "$out" ;;
      *) echo "other" >> "$out" ;;
    esac
    n=$((n + 1))
    printf '\033c'
    if [ "${2}" = tabbed-drift ] && [ "$n" -eq 3 ]; then
      cat "$dialog"
      while IFS= read -r -n1 -s key; do echo "late" >> "$out"; done
    fi
    tabbed_frame "$n"
  done
  sleep 3600
fi

cat "$dialog"
IFS= read -r -n1 -s key
printf '%s' "$key" >> "$out"
if [ "${2:-}" = sticky ]; then
  printf '\033c'
  cat "$dialog"
  IFS= read -r -n1 -s key
  printf '%s' "$key" >> "$out"
fi
printf '\033c'
cat "$root/session/testdata/dialog_presence/idle_composer.txt"
sleep 3600
