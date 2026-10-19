#!/usr/bin/env bash
# A fake AskUserQuestion pane for the structured-question e2e (session:guidance-structured).
# Draws the captured Claude Code dialog, records the ONE key it receives into $1, then redraws
# the idle composer, like the agent moving on. Dialog and composer are the unit-test fixtures.
set -u
root="$(cd "$(dirname "$0")/../../.." && pwd)"
out="$1"
: > "$out"
cat "$root/session/testdata/reply_dialogs/question_single_select_3opts.txt"
IFS= read -r -n1 -s key
printf '%s' "$key" > "$out"
printf '\033c'
cat "$root/session/testdata/dialog_presence/idle_composer.txt"
sleep 3600
