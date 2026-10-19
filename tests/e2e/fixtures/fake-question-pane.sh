#!/usr/bin/env bash
# A fake AskUserQuestion pane for the structured-question e2e (session:guidance-structured).
# Draws the captured Claude Code dialog, records the ONE key it receives into $1, then redraws
# the idle composer, like the agent moving on. Dialog and composer are the unit-test fixtures.
# With "sticky" as $2 the first key leaves the dialog on screen (the agent ignored it) and only a second
# key closes it; $1 then holds every key received, in order.
set -u
root="$(cd "$(dirname "$0")/../../.." && pwd)"
out="$1"
dialog="$root/session/testdata/reply_dialogs/question_single_select_3opts.txt"
: > "$out"
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
