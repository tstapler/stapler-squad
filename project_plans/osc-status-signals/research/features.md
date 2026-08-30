# Research: Existing Status-Detection Architecture & Integration Points

Scope: how session/detection resolves conflicting signals and handles debounce
today, and what that implies for making OSC title/progress a high-priority,
debounce-bypassing signal per AC5/AC6.

## 1. The detection pipeline, end to end

```
tmux control mode "%output %PANE_ID DATA"     (session/tmux/control_mode.go:380)
        │  raw bytes, unmodified — tmux forwards the pane's literal output,
        │  INCLUDING any OSC escape sequences the child process wrote.
        ▼
PTYAccess.Write(data)  →  CircularBuffer        (session/pty_access.go:33)
        │  raw bytes stored as-is; nothing strips OSC at write time.
        ▼
PTYAccess.GetRecentOutput(n) / GetRecentOutputInto  (session/pty_access.go:86-104)
        │  callers: ClaudeController.GetIdleState/GetCurrentStatus,
        │  IdleDetector.DetectState (deprecated path), ratelimit.PTYConsumer.
        ▼
filterTmuxMetadata(tail)   (session/claude_controller.go:716)
        │  strips lines starting with "[" (tmux status-bar text), NOT OSC —
        │  this is unrelated to the OSC-strip in ansiStripRegex.
        ▼
PTYNormalizer.Normalize = stripANSI(collapseCarriageReturns(content))
        │  (session/detection/normalizer.go:11, detector.go:141)
        │  stripANSI's ansiStripRegex (detector.go:129) matches and DISCARDS
        │  OSC sequences (`\x1b\][^\x07]*\x07`) here — this is the exact
        │  point named in the requirements as throwing away title content.
        ▼
PatternSet.MatchLines(text, rawPTY)   (session/detection/pattern_set.go:69)
        │  fixed priority chain over the ALREADY-STRIPPED text; rawPTY is only
        │  used for hasScreenOverwrite() (CR / cursor-up heuristic), not OSC.
        ▼
DetectedStatus  →  IdleDetector.mapStatusToIdleState (session/detection/idle.go:185)
        │  debounce gate: DebounceDelay (default 500ms) — see below.
        ▼
IdleState (Active/Waiting/Timeout) exposed via GetIdleState/GetStatusAndIdleInfo
```

Key file:line references:
- OSC stripped, content discarded: `session/detection/detector.go:129` (`ansiStripRegex`), used by `stripANSI` (`detector.go:141`) and `PTYNormalizer.Normalize` (`normalizer.go:11`).
- Claude text-pattern detector: `session/detection/binaries/claude.go` (pure regex sets, no OSC awareness).
- Pipeline wiring PTY→detector: `session/claude_controller.go:890-932` (`GetIdleState`), `:955+` (`GetStatusAndIdleInfo`), `session/detection/idle.go:151` (`DetectStateFromContent`).
- Registry of per-binary detectors: `session/detection/registry.go` (`DefaultRegistry()`), consumed via `session/detection/binary_detector.go`'s `DetectorRegistry.Lookup`.
- Raw bytes survive tmux control mode intact: `session/tmux/control_mode.go:380` (`%output` case), `session/external_tmux_streamer.go:288-360` (`readControlMode`, debounces `%output` events into `capture-pane` calls but does not touch the byte payload itself for the control-mode path — the PTY write path at `pty_access.go:33` is what actually stores bytes for non-tmux/native sessions).

## 2. How priority/conflict-resolution already works — `PatternSet.MatchLines`

`session/detection/pattern_set.go:69-141` is a **strict, hardcoded priority chain**, not
a scored/weighted system: Error > TestsFailing > NeedsApproval > InputRequired >
readline-typing (special-cased, not a category) > WaitingForAgent > Success > Active >
Processing > screen-overwrite fallback > Idle > Ready (catch-all, returns
`StatusUnknown`). Each category is a first-match-wins scan over compiled regexes in
declaration order (`ps.patterns.<Category>[i]`), no numeric priority field is
actually consulted at match time despite `StatusPattern.Priority` existing in the
struct (`dtypes/dtypes.go:11`) — priority is implicit in the hardcoded chain order,
and the `Priority` int field appears to be documentation/ordering-within-category
only, not read anywhere in `MatchLines`. **This matters for AC5**: "OSC wins over
conflicting text status" cannot be expressed as "just add another category" and
expect it to fall into the right slot — the chain is a fixed sequence of `for`
loops, so an OSC check needs to be its own priority tier, most naturally inserted
as a short-circuit *before* `MatchLines` is even called (at the `detectFromText`
/ `Detect` level in `detector.go:249-278`), not as one more entry inside it.

There is exactly one precedent for a check that already runs *outside* the regular
category loop and short-circuits it: `hasScreenOverwrite(rawPTY)` at
`pattern_set.go:125-127`, inserted between Processing and Idle. It's structurally
the closest existing analog to what OSC detection needs (a raw-byte signal
consulted alongside/instead of the stripped-text categories) — but it only ever
*adds* a StatusExecuting result into the middle of the chain; it doesn't override
higher-priority categories the way AC5 wants OSC to override e.g. a stale idle
prompt in scrollback.

`readlineTypingRegex` (`detector.go:157`, checked at `pattern_set.go:95`) is the one
existing case of a signal deliberately placed to **override a stale marker
elsewhere in scrollback** — its own comment says exactly this: "Checked before
Success so a stale ✻ completion marker in scrollback does not override the current
'user is typing' state." This is the closest conceptual precedent for AC5's
"false-idle" scenario (stale/misleading text pattern vs. a more current signal) and
is worth mirroring in design/tone, though it operates within the same text-based
scan rather than as an out-of-band signal.

## 3. How debounce/stabilization works today — and what "bypass" would touch

Debounce lives in `IdleDetector`, not in `PatternSet`/`StatusDetector`. Both
`DetectState` (deprecated) and `DetectStateFromContent`
(`session/detection/idle.go:121-181`) share this pattern:

```go
newState := id.mapStatusToIdleState(status)
if newState != id.currentState {
    if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= id.config.DebounceDelay {
        id.currentState = newState
        id.lastStateChange = id.timeNow()
    }
}
return id.currentState
```

`DebounceDelay` defaults to 500ms (`DefaultIdleDetectorConfig`, `idle.go:34`). Note
the debounce is asymmetric/lenient already: it only *blocks* a transition when
`currentState != IdleStateUnknown` AND the delay hasn't elapsed; the very first
transition out of `Unknown` is never debounced. There is **no existing "priority
signal bypasses debounce" mechanism** — every `DetectedStatus` computed by
`PatternSet.MatchLines` is treated identically by `mapStatusToIdleState` regardless
of which category matched or how it was derived. This means:

- AC6 ("OSC-derived transitions bypass debounce") requires either (a) a new
  code path in `IdleDetector` parallel to `mapStatusToIdleState` that writes
  `currentState`/`lastStateChange` unconditionally when the status originated from
  OSC, or (b) threading a `bypassDebounce bool` (or a distinct
  `DetectedStatus`-adjacent signal) from `detectFromText`/`Detect` through to
  `DetectStateFromContent`'s debounce check. Both require a new parameter/return
  value on the `Detect*` family, since today `DetectedStatus` alone carries no
  provenance (text-pattern vs. OSC vs. screen-overwrite) once it leaves
  `PatternSet.MatchLines`.
- `hasScreenOverwrite`'s result already flows through the *same* debounce as
  every other status — it is not currently a bypass precedent, just a source
  precedent. Confirmed by reading `mapStatusToIdleState`: it switches purely on
  `DetectedStatus` value (`StatusExecuting`, etc.), with no side channel for
  "how was this derived."

## 4. Rate-limit detector — a structurally separate, non-debounced parallel detector

`session/detection/ratelimit/` (`detector.go`, `manager.go`, `integration.go`,
`scheduler.go`, `recovery.go`) is **not** integrated into `PatternSet`/`IdleDetector`
at all — it's a fully separate `Detector` type with its own regex sets
(`defaultRateLimitPatterns`, `defaultContinuePatterns`, `defaultTimestampPatterns`,
`ratelimit/detector.go:70-96`) and its own `Manager`/`PTYConsumer`
(`ratelimit/integration.go:85-100`) that polls the same `BufferReader` interface
(`GetRecentOutput(n) []byte`) independently, on its own 500ms poll interval, and
calls an `onDetection(Detection)` callback directly — bypassing `IdleDetector`'s
debounce and `PatternSet`'s category chain entirely. **This is the strongest
existing precedent for "OSC gets its own detector that doesn't go through the
text-pattern debounce path"**: rather than threading a bypass flag through
`IdleDetector`, an OSC title/progress detector could be built as a structurally
parallel component (own regex/parse step, own state, own callback into whatever
consumes `DetectedStatus`), consuming the same `GetRecentOutput`/`BufferReader`
interface already used by three independent consumers today. The tradeoff: rate
limit detection currently does NOT feed back into `StatusDetector`'s
`DetectedStatus` enum or `IdleState` — it's presentation-adjacent (drives its own
UI banner/recovery flow), not a `DetectedStatus` override. OSC's requirement (AC5:
override the *same* status enum used by text patterns) is a stronger integration
requirement than what ratelimit does today, so ratelimit is a partial precedent for
"independent parallel detector," not a full precedent for "overrides the shared
status."

## 5. Existing braille-spinner and completion-glyph precedent (why AC3/AC4 are easy on the regex side)

- Braille spinner range already has two precedents to draw from: OpenCode's
  detector uses `[⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋]` literal class (`binaries/opencode.go:69-70`, explicitly
  named `opencode_braille_spinner`), and Claude's own `progress_indicators`
  pattern (`binaries/claude.go:139`) matches a literal Braille-range subset among
  other progress glyphs. Neither uses the full `\x{2800}-\x{28FF}` Unicode block —
  they hand-list the frames actually observed. AC3 asks for the full block; that's
  a straightforward `[\x{2800}-\x{28FF}]` regex, more permissive than existing
  patterns, applied to OSC title text specifically (not general screen text, so
  false-positive risk from the wider range is lower — OSC titles are short,
  single-purpose strings).
- The `✳` (U+2733) idle-glyph is not currently referenced anywhere in
  `session/detection` (verified via grep across `session/**/*.go`) — this is new
  vocabulary for this feature, not an extension of an existing pattern. Note
  `✻`/`✽`/`✶` (different but visually similar asterisk-family glyphs) ARE used
  extensively in `claude.go`'s `claude_thinking_verb`/`verb_duration_completion`
  patterns for *screen* text — worth double-checking against herdr's reference
  implementation that `✳` (not `✻`) is really the idle glyph Claude Code emits in
  the *OSC title specifically*, since a transcription slip between visually
  similar asterisk glyphs (✳ U+2733 vs ✻ U+273B vs ✽ U+273D) would silently break
  AC4 matching.

## 6. Edge cases surfaced by reading the pipeline (not just imagined)

1. **Multiple OSC titles per PTY read chunk.** `GetRecentOutput`/`GetRecentOutputInto`
   return whatever's currently in the circular buffer tail — a single "read"
   for detection purposes can span many terminal frames/writes, so multiple
   `\x1b]0;...\x07` sequences can legitimately appear in one chunk (spinner
   frame N, then frame N+1, then idle). The existing `ansiStripRegex` is a
   global-replace regex (`ReplaceAllString`), so today all of them are silently
   discarded with no ordering signal preserved. Any OSC-capture implementation
   must decide "keep last" (most current title wins) vs. "keep all, let caller
   decide" — "last" is almost certainly correct given every other piece of this
   pipeline (tail-based hashing/caching in `claude_controller.go`, `GetIdleState`)
   treats "most recent state" as authoritative.
2. **Partial/truncated OSC sequences split across reads.** The circular buffer
   is written in whatever chunks the PTY/tmux control-mode delivers
   (`PTYAccess.Write`, `handleOutputBytes` in `control_mode.go:637`) — no
   guarantee an OSC sequence isn't split mid-escape across two `%output` events
   or two `Write` calls. `ansiStripRegex`'s BEL-terminated OSC alternative
   (`\x1b\][^\x07]*\x07`) already silently fails to strip a truncated OSC (no
   terminating BEL in this chunk) — that half-sequence just survives into
   `text` as literal bytes today (a latent, currently-invisible bug in
   `stripANSI`, since nothing depends on 100% OSC removal for correctness, just
   "doesn't break pattern matching too badly"). An OSC-title extractor sitting
   on top of the **tail slice** (not the full incremental byte stream) has the
   same problem in reverse — a title present in the buffer may have its opening
   `\x1b]0;` cut off by the tail-window boundary. Since detection already
   operates on a bounded tail (`StatusDetectionTailBytes = 4096`,
   `statusDetectionTailBytes` used in `claude_controller.go`), a truncated title
   at the tail's leading edge is a real, not hypothetical, failure mode — needs
   an explicit "found closing BEL/ST but no matching opening sequence in this
   window → treat as no title" rule, not "found opening but no closing → wait
   for more data" (there's no streaming buffer to wait on at the point where
   `GetRecentOutput` tail-slicing happens).
3. **Non-Claude-Code OSC titles (nested shell / other app in the pane).** Nothing
   in the current pipeline distinguishes "which process wrote this OSC title" —
   OSC title-setting is a shell/terminal convention, so a user running `git
   log`, `htop`, or just having their shell's own `PROMPT_COMMAND` set a title
   (common in bash/zsh configs) inside the Claude Code pane (e.g. a
   backgrounded shell command, or after Claude Code exits and the pane returns
   to a raw shell) would inject an unrelated title. Given `BinaryDetector` is
   already keyed by binary name (`ClaudeDetector.Name() == "claude"`,
   `DefaultRegistry()`), OSC parsing should probably live behind the same
   per-binary dispatch rather than as a global always-on signal, and the
   Claude-specific glyph checks (Braille block / `✳`) are actually a reasonable
   *implicit* filter here — a nested shell's OSC title is very unlikely to
   contain a bare Braille-range character or U+2733, so requiring the specific
   glyphs (not just "any OSC title present") is itself a defense against
   false-positives from unrelated OSC emitters. Worth stating explicitly as a
   design decision rather than an accidental side effect.
4. **Stale OSC titles left over after the process exits.** OSC title-setting has
   no "clear" convention most shells use by default — once a title is set it
   typically persists in the emulator/pane until something else changes it.
   If Claude Code exits (or is killed) while its last-emitted title still shows
   a Braille spinner, and nothing subsequently overwrites the title (e.g. the
   pane sits at a bare shell prompt with default `PROMPT_COMMAND`/no title
   updates), a naive "last title wins" cache would report `StatusExecuting`
   forever. Given `IsStarted()`/session lifecycle already exists on
   `ClaudeController` (`claude_controller.go:820`), any OSC-derived status
   should be gated by "is the underlying process/session actually still
   running" — same category of staleness problem that `InitializeFromTimestamp`
   already defends against for `lastActivity` after a server restart
   (`idle.go:342-372`, rejects timestamps >24h old or in the future) — a
   similar staleness/liveness gate is the natural analog here, though the
   trigger is process-exit rather than elapsed time.
5. **tmux rewriting/absorbing the OSC title.** Two distinct tmux behaviors are
   relevant and easy to conflate: (a) tmux's own **status-bar** window title
   (governed by `automatic-rename`/`set-titles` tmux options) is a *separate*
   piece of state tmux maintains about the pane, shown in tmux's status line —
   this is what `filterTmuxMetadata` (`claude_controller.go:716`) strips as
   `[session-name]`-style lines, and is NOT the same bytes as the child
   process's OSC sequence; (b) whether tmux **forwards** the child's raw OSC
   bytes through to a control-mode client depends on tmux's `set-titles`
   passthrough behavior — confirmed structurally in this codebase that
   `%output` in control mode carries the pane's literal byte stream
   (`control_mode.go:352,380`, comment: "Terminal output from pane (always
   broadcast, even inside response)"), so as observed in this repo's own
   control-mode client, OSC bytes are NOT swallowed by tmux control mode — they
   arrive as part of `%output`'s DATA payload like any other escape sequence.
   This should be verified empirically (a live tmux pane test) before relying
   on it, but nothing in `control_mode.go`'s parsing suggests tmux filters OSC
   out of the forwarded stream. The one thing NOT yet confirmed by reading code:
   whether tmux's own `set-titles on` (if enabled in the bundled tmux config)
   causes tmux to *itself* re-emit a translated/wrapped OSC sequence to the
   *outer* terminal (the one hosting `tmux attach` or the control-mode client)
   that could shadow or duplicate the inner one — worth checking
   `bootstrap`/tmux config or `.claude/docs/bundling-tmux.md` for whatever tmux
   config ships with this project.

## 7. Likely unstated needs beyond the literal ACs

- **Other binaries eventually wanting the same treatment.** The registry
  (`session/detection/registry.go`, `binary_detector.go`) is already structured
  per-binary (`BinaryDetector` interface: `Name()`, `Patterns()`,
  `FilterContent()`), and Gemini/OpenCode/Aider/Agy each have their own spinner
  conventions already (e.g. OpenCode's own braille pattern). If OSC parsing is
  built as a bolt-on specific to `claude.go` (e.g. a new method only on
  `ClaudeDetector`), it will not generalize; if it's added as an optional new
  method on the shared `BinaryDetector`/`dtypes.BinaryDetector` interface (e.g.
  `ParseOSCTitle(title string) (DetectedStatus, bool)` with a no-op default for
  binaries that don't implement it), the non-goal "extending to binaries other
  than Claude Code" is respected today while leaving a natural seam for later.
  Worth flagging as a design choice for the plan phase rather than deciding it
  in research, but the registry shape clearly anticipates per-binary variation.
- **UI wanting to show the raw OSC title as a tooltip/debug aid.**
  `DetectionEvent` (`session/detection/events.go:9-16`) already carries a
  `TailSnippet` (cleaned/stripped text) surfaced to the frontend via
  `DetectionEventsPanel.tsx`, and `StatusBadge.tsx:92` already renders a
  `context` string (from `DetectWithContext`'s pattern `Description`) as a
  tooltip. There's a ready-made precedent and plumbing seam for adding a raw
  (or last-parsed) OSC title string alongside `MatchedPattern`/`TailSnippet` in
  `DetectionEvent`, which would let a future UI surface "Claude's window title
  says: ⠋ (thinking)" for debugging false-idle reports — not required by the
  ACs, but cheap to add given `DetectionEvent`'s existing shape and would
  directly help debug the exact false-idle scenario this feature exists to fix.
- **Progress (`\x1b]4;0`) is explicitly scoped down** in the requirements
  ("beyond completion signal... unless research finds richer data worth
  using") — nothing in this codebase currently parses or references OSC 4
  (color-palette-set, which is what `\x1b]4;` normally means in the wider OSC
  spec; note this project's use of "OSC progress" for `\x1b]4;0` doesn't match
  the standard OSC 4 semantics — worth flagging that terminology precisely in
  the plan phase, and confirming against herdr's actual sequence numbers, since
  ConEmu/Windows Terminal's progress OSC is code **9** (`\x1b]9;4;...`), not 4 —
  this project's requirements doc may be using a herdr-specific or
  possibly-mistaken sequence number that should be verified against herdr's
  source before implementation, not just trusted from the requirements text).

## Summary of integration recommendation surfaced by this research

- The natural insertion point for OSC capture is a **new raw-byte pass parallel
  to `stripANSI`**, not a new category inside `PatternSet.MatchLines` — mirror
  `hasScreenOverwrite`'s "operate on rawPTY before it's discarded" structure
  (`pattern_set.go:125`), but make it a short-circuit consulted by
  `detectFromText`/`Detect` (`detector.go:249-278`) BEFORE the category chain
  runs, since AC5 needs it to *override*, not just supplement.
- Debounce bypass (AC6) has no existing "skip debounce" precedent inside
  `IdleDetector` — closest analog is the ratelimit package's structurally
  separate detector that never goes through `IdleDetector` at all. Cleanest
  path is likely: `DetectedStatus` gets a provenance/priority signal threaded
  through to `DetectStateFromContent`, OR OSC-derived state writes
  `currentState`/`lastStateChange` unconditionally via a new method that
  bypasses `mapStatusToIdleState`'s debounce gate entirely.
- Per-binary extensibility (AC2 "reaching claude.go's detector") should ideally
  go through `dtypes.BinaryDetector` as an optional interface addition, not a
  Claude-only special case, given the registry already anticipates per-binary
  variation and three other binaries have their own spinner conventions.
