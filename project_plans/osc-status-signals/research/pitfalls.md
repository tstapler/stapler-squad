# Research: Pitfalls — OSC Title/Progress Sequences as Status Signals

Scope: what commonly goes wrong parsing OSC escape sequences from a live PTY
byte stream in this repo's tmux-control-mode architecture, per the 7
questions in the research brief. Research only — no fix design.

---

## 1. Does tmux intercept/rewrite/pass-through OSC 0/1/2? (biggest risk — tested live)

**Confirmed empirically, not just from docs.** Live experiment against the
repo's actual `tmux 3.6a` binary (commands and full output preserved below):

```bash
tmux new-session -d -s osc-cm3 -x 80 -y 24
# open a real control-mode client (tmux -C attach) with a FIFO held open on stdin
# so the client doesn't detach on EOF
tmux send-keys -t osc-cm3 "printf '\033]0;TEST-TITLE-SPINNER\007hello world\n'" Enter
```

Result, captured from the control-mode client's raw stdout:

```
%output %81 \033]0;TEST-TITLE-SPINNER\007hello world\015\012
```

and separately:

```
$ tmux display-message -t osc-cm3 -p '#{pane_title}'
TEST-TITLE-SPINNER
```

**Two things are both true simultaneously:**
1. tmux's own VT100 emulator parses OSC 0/2 and updates `#{pane_title}`
   (consumed internally, drives things like the tmux window-list title).
2. tmux **also** relays the raw, unmodified OSC byte sequence
   (`\x1b]0;...\x07`, verbatim including the terminator) through `%output`
   to every subscribed control-mode client. This is *not* documented tmux
   behavior I could find written down anywhere in this repo's comments —
   it had to be verified live, which is why this was flagged as the
   biggest feasibility risk.

**Why this makes sense architecturally**: control mode is designed so that
each client (visible pane rendering aside) runs its own terminal emulator
over the raw byte stream — tmux doesn't try to pre-render the screen for
control-mode clients, it hands them the same bytes the pane's own vt100
parser sees. `session/tmux/control_mode.go`'s `handleOutputBytes` /
`decodeControlModeOutput` (`session/tmux/control_mode.go:636-678`) already
decodes tmux's octal-escaped `%output` format into raw bytes, and
`session/detection/detector.go`'s `ansiStripRegex`
(`\x1b\][^\x07]*\x07` at line 129) already strips these exact OSC sequences
from that raw text today — which independently confirms the OSC bytes are
already reaching the detection layer today, just being discarded.

**Feasibility verdict: the feature is feasible as designed.** OSC title
content is observable at the `%output` control-mode layer with zero tmux
configuration changes (no `allow-passthrough`, no special tmux option
needed — that option only matters for *terminal→terminal* passthrough of
things like Kitty graphics/hyperlinks from *inside* tmux to the outer
terminal; it is irrelevant here since we're reading `%output`, not
rendering to an outer terminal).

**One thing NOT tested**: whether `capture-pane` (non-control-mode,
polling path — `grep -rn "capture-pane" session/` shows no direct Go
callers outside tests, so the codebase's primary path is control mode) can
observe OSC content. It cannot in any useful way — `capture-pane -p`
renders the *screen*, and window-title OSC sequences do not affect any
visible cell, so they are invisible to plain-text `capture-pane` output
(confirmed: the OSC bytes never appeared in `capture-pane -p` in the same
experiment, only `#{pane_title}` and the raw `%output` stream carried it).
**Implication for design: OSC capture must hook the control-mode
`%output` raw-byte path (`session/tmux/control_mode.go` →
`session/instance_tmux.go:552`'s `SubscribeToControlModeUpdates` /
`session/native_process_manager.go`'s direct PTY read loop for the non-tmux
path), not any `capture-pane`-based text path.**

---

## 2. Partial/split OSC sequences across read boundaries

**Confirmed as a real, not theoretical, risk — also caught live in the
same experiment.** The control-mode `%output` stream for the *interactive
keystroke echo* (as `tmux send-keys` typed the `printf` command one
key-injection at a time) arrived as a long sequence of single-character
`%output` lines:

```
%output %81 p
%output %81 \010pr
%output %81 i
%output %81 n
%output %81 t
%output %81 f
%output %81  
%output %81 '
%output %81 \134
%output %81 0
%output %81 3
%output %81 3
%output %81 ]
%output %81 0
%output %81 ;
...
```

Contrast with the *actual* OSC sequence written by `printf`'s single
`write()` syscall, which arrived intact in one `%output` message:

```
%output %81 \033]0;TEST-TITLE-SPINNER\007hello world\015\012
```

So: whether a given OSC sequence arrives whole or fragmented depends
entirely on (a) how the writing process chose to `write()` it — Claude
Code's actual title-setting write is unlikely to be split by the child
process itself, but (b) how the PTY/kernel/tmux chunk delivery under load,
which is not guaranteed. The **generic mechanism** that can fragment a
sequence in this codebase:

- `session/native_process_manager.go:450` reads the raw PTY in
  `buf := make([]byte, 4096)` chunks — a 4096-byte read boundary landing
  mid-OSC-sequence would split it exactly like the keystroke-echo case
  above did, just for a different reason (buffer-size boundary instead of
  per-key delivery).
- `session/detection/idle.go` / `session/detection/detector.go`'s
  `StatusDetectionTailBytes = 4096` (`detector.go:241`) tail window is a
  **hard truncation boundary**: if an OSC title sequence straddles the
  edge of the last-4096-bytes window (e.g. mid-turn, right as new output
  pushes the window forward), the captured tail could contain only the
  back half of a title (`...NER\007hello world`) with no opening
  `\x1b]0;`, or only the front half with no terminator.

**Implication for design**: any OSC extraction logic needs to be a
stateful scanner across the accumulated/tail buffer (find the *last*
complete `\x1b]...(\x07|\x1b\\)` in the available window), not a per-chunk
regex applied to each individual `%output`/read() delivery in isolation —
the current `ansiStripRegex` approach (regex over one already-assembled
text blob) works today because it's applied to the reassembled tail
buffer, not per-chunk; the same discipline must carry over to OSC
extraction.

---

## 3. Malformed/unterminated OSC sequences — buffering and ReDoS risk

`ansiStripRegex`'s OSC branch is `\x1b\][^\x07]*\x07` — bounded by
"anything that is not BEL, then BEL." This is **not** vulnerable to
catastrophic backtracking (no nested quantifiers, `[^\x07]*` is a single
linear scan), so classic ReDoS is not a concern for the existing pattern
or a similar OSC-title-extraction pattern built the same way.

The real risks are different:

- **Unterminated sequence with no BEL/ST at all** (e.g. output truncated
  mid-write, or the tail-window boundary from §2 cuts off the terminator):
  `[^\x07]*\x07` fails to match and the *entire* rest of the buffer up to
  end-of-input is silently treated as "not an OSC sequence" — for
  `ansiStripRegex`'s stripping use case this just means the raw escape
  bytes leak into the "stripped" text (a cosmetic/pattern-matching
  nuisance, already possibly happening today unnoticed). For a *new* OSC
  title extractor, the equivalent failure mode is "title never observed
  this frame" — which is actually the safe failure (falls through to
  requirement 7's text-pattern fallback), **provided** the extractor
  doesn't try to buffer indefinitely waiting for a terminator that a
  buggy/adversarial process might never send. Cap however much is
  buffered while waiting for a terminator (the existing 4096-byte tail
  window is a natural, already-enforced cap — reuse it rather than
  inventing a second one).
- **ST-terminated OSC (`\x1b\\`) vs BEL-terminated (`\x07`)**: the current
  `ansiStripRegex` only recognizes BEL (`\x07`) as a terminator. Some
  terminal apps/libraries emit `ESC \` (String Terminator, 2 bytes)
  instead of BEL for OSC sequences (this is the "modern"/xterm-spec form).
  If Claude Code (or a future version of it, or the Node/Ink TUI framework
  it's built on) ever switches terminators, `[^\x07]*\x07` would fail to
  match ST-terminated sequences at all, and everything from that OSC
  through the *next* BEL anywhere later in the stream would be
  misinterpreted as "inside" the OSC content. **Worth handling both
  terminators explicitly** in any new OSC-title parser, not assuming BEL
  only (this is a correctness gap in the *existing* `ansiStripRegex` too,
  inherited by any code that copies its pattern).

---

## 4. False positives: spinner/✳ chars in OSC titles from unrelated programs

This risk is **real and not fully mitigated by "these are Claude-specific
glyphs"** — grep of `session/detection/binaries/claude.go` shows the
Braille spinner block (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`, U+2800–28FF subset) and `✳` are
**already** used as *text-pattern* signals today (`progress_indicators`
pattern line 139, `claude_thinking_verb` pattern line 127) — meaning these
glyphs are not unique to Claude Code's OSC title; they are the same
generic "braille dot spinner" and "eight-spoked-asterisk" characters used
by countless other CLI spinner libraries (ora/cli-spinners' `dots` frame
set is exactly U+2800-range Braille; many shells/tools use `✳`-family
glyphs from the same Unicode "dingbat"/spinner conventions Claude Code
itself draws from). Two concrete false-positive vectors specific to this
repo's architecture:

- **Nested sessions**: a Claude Code session running inside a tmux pane
  that itself shells out to another program which sets its own OSC title
  with a Braille spinner (e.g. a nested `npm install` progress spinner, a
  nested SSH session to a host whose shell sets a spinner-decorated
  title) would produce an OSC title with the same glyph class, attributed
  to the wrong (outer) session.
  - Worktrees in this repo already have this exact shape live —
    `.claude/worktrees/agent-*/` are themselves Claude Code sessions that
    could spawn nested tool invocations.
- **Non-Claude binaries entirely**: any binary run inside a Stapler Squad
  session (not just Claude Code) could theoretically set an OSC title with
  these glyphs. Per the requirements' explicit non-goal ("extending to
  binaries other than Claude Code"), this is scoped away for now, but it
  means the OSC-title detector **must be gated on knowing the session is
  actually running the `claude` binary** (mirroring how
  `binaries.ClaudeDetector` is only invoked for Claude sessions via
  `session/detection/registry.go`'s per-binary dispatch) — an ungated OSC
  detector watching *any* session's title would misfire on nested/nom-Claude
  processes constantly.

**Mitigation direction (not a full design, just what the pitfall implies)**:
the OSC title signal is inherently weaker evidence of "this specific
process's state" than text-pattern matching against the actual rendered
pane content, because a title can be set by *any* process that currently
owns the terminal, including a child process temporarily in the
foreground. Any implementation should treat an OSC title lacking the
expected format entirely (no spinner, no `✳`) as "no signal" (fall
through to text patterns per requirement 7) rather than actively
overriding to Idle — asymmetric trust, since a false "the title says
executing" is lower-cost (a false busy indicator self-corrects on the
next real idle title) than a false "the title says idle" incorrectly
overriding a text pattern that says the model is actively mid-tool-call.

---

## 5. Debounce-bypass risk — reintroducing status flapping

`session/detection/idle.go`'s `IdleDetector.DetectState()` /
`DetectStateFromContent()` (lines 121-181) gate every state transition on:

```go
if newState != id.currentState {
    if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= id.config.DebounceDelay {
        id.currentState = newState
        id.lastStateChange = id.timeNow()
    }
}
```

`DebounceDelay` defaults to 500ms (`DefaultIdleDetectorConfig`,
`idle.go:34` — already reduced once from 2s "for faster response," per its
own comment, implying this knob has been tuned before and is sensitive).
This is the **single choke point** any "OSC bypasses debounce" design must
modify or route around.

**This repo has a documented, hard-won history of exactly this failure
class** (`project_plans/backlog-session-thrashing/research/pitfalls.md`),
though that history is about a *different* subsystem (autonomous-worker
turn budgets/respawn loops, not PTY status flapping specifically). The
transferable lesson, stated explicitly in that doc's §3 and repeated in
its §6 generic-pitfalls list: **this codebase has repeatedly created a
second, uncoordinated timing/threshold mechanism that fights an existing
one**, and every time that has happened it produced a live incident (the
78-bounce loop from `dd3a287f`/#180, the `BUG-043` cross-gate coordination
gap, the "three different staleness thresholds" pattern flagged in
`review-gate-stale-session-rework/research/pitfalls.md`). Applied to this
feature specifically:

- Adding a **second, OSC-only transition path that skips
  `DebounceDelay` entirely** is structurally the same shape as those prior
  incidents: two paths (text-pattern-debounced, OSC-immediate) writing to
  the same `id.currentState`/`id.lastStateChange` fields, racing each
  other. If Claude Code's OSC title itself flickers rapidly between the
  spinner and `✳` (plausible — the spinner glyph rotates on every
  animation frame, some of which coincide with genuinely-brief idle gaps
  between tool calls, e.g. between two rapid-fire tool calls in the same
  turn), an *undebounced* OSC path would reproduce flapping at the OSC
  layer even though the original text-pattern debounce is untouched and
  working correctly.
- The requirements doc's own acceptance criterion 6 already anticipates
  this ("OSC-derived transitions bypass debounce... **or documented
  reason given if infeasible**") — the pitfall this history predicts is
  that "bypass debounce" for OSC could still need *its own*, differently
  tuned debounce (e.g. require the OSC-derived status to persist across
  ≥2 consecutive `%output` observations before firing, distinguishing
  "real state change" from "single animation frame") rather than truly
  zero debounce — otherwise this becomes the "fifth uncoordinated clock"
  pattern the sibling research explicitly warns against by name.
- Concretely: `mapStatusToIdleState` (idle.go:185) already treats
  `StatusExecuting`/`StatusProcessing` as `IdleStateActive` and mutates
  `id.lastActivity` on every call where that mapping fires — an
  undebounced OSC path calling into this same function at high frequency
  (e.g. once per `%output` frame containing a spinner-titled OSC) would
  churn `id.lastActivity` far more often than the text-pattern path does
  today, which changes the behavior of every *other* consumer of
  `lastActivity`/`GetIdleDuration()` (idle-timeout logic,
  `IdleStateTimeout` in `mapStatusToIdleState`'s `StatusIdle`/`StatusReady`
  branch) even when nothing about idle-timeout was intended to change.

---

## 6. Version fragility — Claude Code's OSC format can change

No version-pinning or format-negotiation exists anywhere in
`session/detection/binaries/claude.go` today — patterns are static
regexes with no version gate, and the file's own doc comments (and the
`bug_regression_test.go` history, see §7) show this file has already been
revised multiple times as Claude Code's actual terminal output format
changed release to release (indented spinners, cursor-forward encoding
instead of space, asterisk verb variations). The existing pattern for
tolerating this drift is **breadth of matching, not version detection** —
e.g. `claude_thinking_verb`'s character class
`[·✢✳✶✻✽●*✦]` already covers Claude Code's known historical spinner-glyph
variants (its own comment says "any spinner frame" is intentional), rather
than hardcoding one exact glyph and gating by version string.

**Implication for OSC title parsing**: the same "graceful degradation by
breadth, not by version gate" pattern should apply — if Claude Code's OSC
title format changes (different marker than `✳`, an expanded/different
Braille frame set, or the format is dropped from the title and moved to
OSC progress only), the correct failure mode per requirement 7 is silent
fallback to text-pattern detection, not a hard error or a stuck status.
Concretely this means: the OSC parser should be a narrow, best-effort
classifier (does this title contain *a* Braille char in U+2800-28FF, does
it contain `✳`) rather than an exact-string/exact-sequence match against
one specific title format — an exact-match design would silently stop
firing the moment Claude Code changes its title text even slightly (e.g.
appends a session name, changes capitalization), with no test or runtime
signal that the OSC path went dark, since requirement 7's fallback would
mask it as "text patterns still work fine."

---

## 7. Prior related bug reports / regression tests worth reading first

`session/detection/bug_regression_test.go` and `asterism_test.go` contain
**no existing OSC-specific tests** — grep confirms zero hits for
`OSC`/`title`/`\x1b\]` in either file. However, both files are directly
relevant prior art for *why* the text-pattern layer this OSC feature must
coexist with is shaped the way it is, and each documents a previously-live
false-idle or false-active bug in the same spinner-detection problem
space this feature targets:

- **`TestBug1_IndentedSpinner`** (`bug_regression_test.go:8-34`): a spinner
  indented by leading whitespace was previously invisible to detection
  because the pattern required column-0 anchoring. Generalizes to: OSC
  title text has no "indentation" concept at all (it's a flat string), so
  this specific bug class can't recur in the OSC path, but it's evidence
  that **whitespace/formatting assumptions baked into text patterns have
  broken detection before** — a reason the OSC signal is valuable (it's
  immune to a whole class of screen-rendering-layout bugs) but also a
  reason not to assume the *text* fallback (req. 7) is fully reliable on
  its own when OSC is unavailable.
- **`TestBug_ThinkingWithStillThinkingSuffix`** (`bug_regression_test.go:636`)
  and the "Claude Code v2 encodes the spinner line as `CHAR\x1b[C verb`
  (cursor-forward not space)" comment (`bug_regression_test.go:188`):
  direct precedent for **Claude Code changing its own escape-sequence
  encoding of the spinner between versions** — the exact version-fragility
  concern in §6, already realized once for the screen-rendering encoding
  (not OSC, but the same underlying spinner-glyph-in-escape-sequences
  mechanism). Strong signal that Claude Code's OSC title encoding should
  be expected to drift similarly and the parser should be written
  defensively from day one, not patched reactively after a version bump
  breaks it (as the screen-rendering path was).
- **`TestClaude_ScrollbackFalsePositive_ActiveThenIdle`**
  (`asterism_test.go:123`) and the `filterTmuxMetadata`-related tests
  (`bug_regression_test.go:803-830`): both document that **stale content
  lingering in a buffer** (old spinner lines still visible in scrollback,
  a stale tmux status-bar fragment) has previously caused false-Active
  detections. Directly relevant to §2/§3's tail-window truncation
  concern — an OSC title captured from a *stale* pane state (if the
  extraction logic reads the wrong slice of the buffer, or caches a
  previously-seen title without freshness tracking) could reproduce this
  exact false-positive class at the OSC layer.

**No existing regression test would have caught an OSC-related bug** —
this confirms acceptance criterion 9 (new test coverage) is filling a
genuine gap, not duplicating existing coverage.

---

## Summary — most important pitfalls

1. **Feasibility is confirmed, not assumed** — live-tested against tmux
   3.6a: `%output` control-mode notifications carry the raw, unstripped
   OSC byte sequence, even though tmux *also* consumes it internally for
   `#{pane_title}`. `capture-pane -p` text does **not** carry it (OSC
   title has no visible screen effect) — the OSC hook must go through the
   control-mode `%output` raw-byte path, not any capture-pane-based path.
2. **Split sequences are a demonstrated, not hypothetical, risk** — the
   same live experiment caught tmux delivering OSC-sequence bytes as
   dozens of single-character `%output` messages under one code path
   (interactive echo) while delivering an identical sequence intact under
   another (a single `write()`). Any parser must be a stateful scanner
   over the reassembled tail buffer, reusing the existing 4096-byte
   `StatusDetectionTailBytes` window as the buffering cap — not a
   per-chunk regex.
3. **Undebounced OSC transitions risk reproducing a failure class this
   codebase has hit repeatedly** (documented across
   `backlog-session-thrashing`, `BUG-043`, `BUG-048`,
   `review-gate-stale-session-rework`'s pitfalls docs): a second,
   uncoordinated transition path writing to the same state
   (`IdleDetector.currentState`/`lastActivity`) as the existing debounced
   path is the exact shape that has caused live bounce-loop incidents
   before, just in a sibling subsystem. "Bypass debounce" likely needs its
   own, differently-tuned confirmation (e.g. persist across ≥2 frames)
   rather than literally zero debounce.
4. **Spinner/✳ glyphs are not Claude-unique** — they're already reused as
   generic text-pattern signals in `claude.go` today, meaning OSC titles
   from nested/unrelated processes using the same common spinner-glyph
   conventions are a real false-positive vector, not a hypothetical one.
   The detector must be gated to only fire when the session's binary is
   known to be `claude` (mirroring the existing per-binary dispatch), and
   should treat "no recognizable pattern in the title" as no-signal
   (fall through) rather than forcing idle.
5. **No existing test coverage would catch an OSC regression** —
   `bug_regression_test.go`/`asterism_test.go` have zero OSC-related
   assertions today, but both document prior, now-fixed false-idle/false-active
   bugs in the *adjacent* text-pattern spinner-detection space (indentation
   sensitivity, encoding changes across Claude Code versions, stale
   scrollback content) — all of which are precedents for failure modes the
   new OSC path should be defensively designed against from the start.
