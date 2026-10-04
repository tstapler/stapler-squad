#!/usr/bin/env bash
# Scripted stand-in for a live `claude` process, used by the
# app-scrollback-forwarding E2E suite (tests/e2e/scroll-forward-*.spec.ts) so
# it never depends on a real `claude` binary in CI -- mirrors REQ-4's
# "golden fixture, no live CLI" strategy (project_plans/app-scrollback-forwarding/
# implementation/validation.md).
#
# Enters the terminal alt-screen buffer (DECSET 1049, required by
# session/scroll_gate.go's AppScrollGate), prints the golden "before" fixture
# verbatim, then on receiving the xterm PageUp sequence -- ESC [ 5 ~, i.e.
# session/gesture_forward_strategy.go's pageUpBytes -- redraws with the golden
# "after" fixture. Every subsequent PageUp redraws the SAME "after" content
# again (byte-identical), which is how session/instance_scroll_forward.go
# computes ScrollForwardOutcome_AT_TOP (before/after byte-identical) -- so one
# press yields DELIVERED and the next yields AT_TOP, matching a real Claude
# Code session hitting the top of its transcript.
set -u

# Resolve through a symlink (the test helper launches this script via a
# `claude`-named symlink so session/instance_tmux.go's isClaude basename
# match resolves it) via readlink -f, not just dirname "$0" -- otherwise
# SCRIPT_DIR would be the symlink's own directory (an ephemeral tmp dir with
# no session/testdata sibling) instead of this file's real location.
REAL_SELF="$(readlink -f "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$(cd "$(dirname "$REAL_SELF")" && pwd)"
BEFORE_FILE="$SCRIPT_DIR/../../../session/testdata/scroll_forward_claude_before.txt"
AFTER_FILE="$SCRIPT_DIR/../../../session/testdata/scroll_forward_claude_after.txt"

redraw() {
  # Clear screen + home cursor, then print the given fixture file verbatim --
  # mirrors a real redraw rather than appending, so a poll-and-compare
  # quiescence check (waitForRedrawQuiescence) sees a clean, stable frame.
  printf '\x1b[2J\x1b[H'
  cat "$1"
  # Trailing bare ">" prompt line -- matches session/detection/binaries/
  # claude.go's "claude_readline_prompt" Idle pattern
  # (`(?m)^>\s*▌?\s*$`), a pure screen-text signal that does not depend on
  # tmux forwarding an OSC title sequence through to control-mode %output.
  #
  # Verified empirically (2026-09-15) that the OSC-title route below does
  # NOT reach session/claude_controller.go's classifyOSC: tmux consumes an
  # OSC 0 sequence to update its own internal pane title but does not
  # replay those bytes into the %output stream ClaudeController's PTYAccess
  # buffer reads, so ansi.ExtractLastOSC never finds it there -- a real
  # gap between this repo's OSC-based idle detection and how tmux
  # control-mode actually behaves, filed as a follow-up (not this
  # feature's to fix; AppScrollGate's StatusIdle check is satisfied by the
  # text-pattern match below regardless). The OSC emission is left in
  # place since it's harmless and documents the intended production
  # signal.
  printf '\n> \n'
  # OSC 0 (set window title) containing the idle glyph U+2733 (✳) -- the
  # signal session/claude_controller.go's classifyOSC/ClassifyOSCTitle
  # looks for (session/detection/binaries/claude.go's oscIdleGlyph). Kept
  # as a secondary signal; see the comment above for why it's not load-
  # bearing in this fixture today.
  printf '\x1b]0;\xe2\x9c\xb3\x07'
}

# Enter the alternate screen buffer before printing anything. This first
# redraw is synchronous (not backgrounded) -- callers (openAndWaitConnected)
# wait for exactly this first byte to know the session is up.
printf '\x1b[?1049h'
redraw "$BEFORE_FILE"

# KEY_RECEIVED_FLAG's existence means a real keystroke (PageUp or Ctrl+L,
# below) has already been processed -- see the backgrounded loop's comment.
KEY_RECEIVED_FLAG="${TMPDIR:-/tmp}/alt-screen-scroll-fixture-key-received.$$"
rm -f "$KEY_RECEIVED_FLAG"

# LOCK_DIR guards every redraw()/live_resume() call below with a portable
# mkdir-based mutex (works on both Linux CI and macOS dev machines -- no
# `flock` dependency) so the backgrounded repeat-loop and the foreground
# key-handler can never write to the shared pty at the same time. Without
# this, a background BEFORE redraw racing a foreground AFTER redraw
# interleaved their printf/cat calls byte-for-byte, producing combined
# output taller than either file alone -- which then scrolled past the
# pane's actual height (found via a real, reproducible failure: the
# server's capture-pane snapshot started mid-file, at the fixture's AFTER
# content's 10th line, even though a real keystroke had already been read
# and its own redraw() call started well before that capture -- the
# 82x22 negotiated pane comfortably fits either file alone, so only two
# interleaved redraws stacked together could push content that far down).
LOCK_DIR="${TMPDIR:-/tmp}/alt-screen-scroll-fixture-lock.$$"
acquire_lock() {
  while ! mkdir "$LOCK_DIR" 2>/dev/null; do
    sleep 0.01
  done
}
release_lock() {
  rmdir "$LOCK_DIR" 2>/dev/null
}

# Re-emit the same entry+redraw a few times, briefly spaced out, IN THE
# BACKGROUND. This still gives the server's several PTY-output consumers
# that attach asynchronously right after tmux session creation (the hub's
# raw-output pump, and ClaudeController's status-detection ring buffer in
# particular) a second chance to observe both the 1049h marker and real
# (idle-looking) content, without hardcoding a single fragile sleep
# duration -- same as before.
#
# Backgrounded (Fix 8, found via a real, reproducible failure in
# scroll-forward-outcome-signals-distinct.spec.ts) rather than run inline
# before the read loop below: inline, this loop delayed the script's own
# readiness to consume a PageUp/Ctrl+L keystroke by up to ~0.9s (3 * 0.3s).
# openAndWaitConnected only waits for the *first* byte of output (long
# before this loop finishes), so a scroll-forward gesture fired shortly
# after it resolves could have its keystroke bytes sit unconsumed in the
# pty's input buffer for that whole window. Meanwhile
# session/instance_scroll_forward.go's waitForRedrawQuiescence -- which only
# checks "are two 150ms-apart polls byte-identical", with no way to know
# whether the pane has actually responded to the keystroke it just sent --
# could observe two consecutive polls of this same loop's own byte-identical
# BEFORE-content repeats and wrongly declare the capture "settled" on that
# stale, pre-keystroke content. The real AFTER redraw then arrived later,
# once this loop finally finished and the read loop drained the buffered
# bytes, as an ordinary output frame the client had no way to associate with
# the (already-sent, stale) AppScrollbackResponse -- TerminalOutput.tsx's
# self-echo check correctly saw it didn't match the stale recorded
# signature and treated it as a genuine live resume, clearing the
# ScrollSourceIndicator banner within milliseconds of it appearing.
#
# Guarded by KEY_RECEIVED_FLAG so a real keystroke, once processed, stops
# this loop from clobbering the redraw it just produced. Checked both before
# AND after acquire_lock: a keystroke can arrive while this iteration is
# already blocked waiting for the lock, so the post-lock check is what
# actually prevents a stale redraw from firing right after a real one.
(
  for _ in 1 2 3; do
    sleep 0.3
    [ -e "$KEY_RECEIVED_FLAG" ] && break
    acquire_lock
    if [ -e "$KEY_RECEIVED_FLAG" ]; then
      release_lock
      break
    fi
    printf '\x1b[?1049h'
    redraw "$BEFORE_FILE"
    release_lock
  done
) &

# scroll-forward-return-to-live.spec.ts (UX-AC-2) needs a deterministic way to
# simulate the live agent producing new output (the real, only mechanism that
# clears TerminalOutput.tsx's ScrollSourceIndicator banner -- see
# handleOutput's Task 1.4.2b: any *normal* output frame while
# appScrollbackActiveRef is true clears it). Ctrl+L (0x0c, a conventional
# "redraw" keystroke, never part of the PageUp sequence) is repurposed here as
# that trigger, distinct from the golden fixture's own AFTER content so a test
# can tell the two apart.
live_resume() {
  printf '\x1b[2J\x1b[H'
  printf 'claude> (resumed) continuing the live session...\n'
  printf '\n> \n'
  printf '\x1b]0;\xe2\x9c\xb3\x07'
}

# Disable canonical mode + local echo only (NOT `stty raw`, which also
# disables output post-processing and would turn our own \n into unindented
# "staircase" output) so single bytes are delivered immediately instead of
# being line-buffered by the kernel tty driver -- required for a 4-byte
# escape sequence with no trailing newline to ever be observed by `read`.
# Started immediately (not after the repeat loop above, which now runs
# concurrently in the background) so this script is ready to consume and
# respond to a keystroke from as close to session start as possible.
stty -icanon -echo min 1 time 0 2>/dev/null

seq=""
while IFS= read -r -n 1 -d '' byte; do
  seq="${seq}${byte}"
  if [ ${#seq} -gt 4 ]; then
    # Keep only the last 4 bytes read (bash substring on a 4-byte-max buffer).
    seq="${seq:1:4}"
  fi
  if [ "$seq" = $'\x1b[5~' ]; then
    : > "$KEY_RECEIVED_FLAG"
    acquire_lock
    redraw "$AFTER_FILE"
    release_lock
    seq=""
  elif [ "$byte" = $'\x0c' ]; then
    : > "$KEY_RECEIVED_FLAG"
    acquire_lock
    live_resume
    release_lock
    seq=""
  fi
done

stty sane 2>/dev/null
rm -f "$KEY_RECEIVED_FLAG"
rmdir "$LOCK_DIR" 2>/dev/null
