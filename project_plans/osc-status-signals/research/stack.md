# Research: Technology Stack for OSC Title/Progress Status Signals

## Question

Which libraries/patterns apply for parsing OSC escape sequences from a PTY byte
stream in Go? What already exists in this repo vs. what needs adding? Is there a
community-standard library, or is hand-rolled parsing the norm for this kind of
narrow task? Can the existing `ansiStripRegex` in `session/detection/detector.go`
be extended to capture rather than discard?

## Summary

**No new dependency is needed.** This repo already has two independent,
battle-tested patterns for scanning ANSI/OSC sequences from raw PTY bytes —
a shared regex-based CSI helper (`pkg/ansi`) and a full byte-level escape-code
state machine with native OSC support (`pkg/analytics`). Neither the Go
ecosystem at large nor this repo's own history reaches for a third-party
terminal-emulation library for this kind of narrow "detect this one escape
sequence in a byte stream" task — hand-rolled regex/byte-scanning is the
established pattern here, consistent with common Go community practice (see
"Community practice" below). The work is almost entirely wiring: capture OSC
title payload during the existing strip pass, thread it as a new field through
`detectFromText`/`MatchLines`, and give it override authority in
`session/detection/binaries/claude.go`.

## What's already in the repo

### 1. `go.mod` — no OSC-aware terminal library present

```
grep -iE "ansi|term|osc|vt10|charm|muesli" go.mod
	golang.org/x/term v0.43.0                                          # raw-mode/pty sizing only, no escape parsing
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c    // indirect
	github.com/moby/term v0.5.2                                        // indirect
```

`go-ansiterm` and `moby/term` are transitive (`// indirect`) and **not imported
by any `.go` file in the repo** (verified via `grep -rl` across all non-worktree
`.go` files — zero hits). They were pulled in by some other dependency (likely
Docker/testcontainers tooling) and are not usable as-is for this feature without
adding a direct dependency and import — not worth it given the narrow, well-
bounded parsing task (two fixed OSC forms: window title `\x1b]0;...BEL` and the
progress code `\x1b]4;0...BEL`).

`golang.org/x/term` only provides raw-mode terminal control and size queries
(`term.MakeRaw`, `term.GetSize`) — it does not parse or interpret escape
sequences at all, so it's irrelevant to this feature.

**Conclusion: no new dependency required.**

### 2. `pkg/ansi` — the repo's existing "single source of truth" for CSI scanning

`pkg/ansi/csi.go` exports `CSIFinalByteClass` (regex character class) and
`IsCSIFinalByte` (byte-range check) specifically so that the *same* ECMA-48
final-byte range (0x40–0x7E) isn't reimplemented (and re-buggy) across
`session/detection`, `session/detection/ratelimit`, `session/tmux`,
`server/services`, and `pkg/analytics` — see the package doc comment, which
cites a historical bug (BUG-025) from exactly that kind of duplication. This
package intentionally does **not** cover OSC — it says explicitly "the CSI
branch's final-byte class... other alternatives (OSC, charset designation,
etc.) need to build their own regexp." This is the established go-to place if
an OSC-equivalent constant (e.g. an `OSCTerminatorClass` or a shared
`ExtractOSCPayload` helper) is worth extracting for reuse. Given the second
existing OSC parser below, extending `pkg/ansi` to add a *shared* OSC-payload
extraction helper (rather than duplicating parsing logic a third time) is worth
considering during planning — see Pitfalls note.

### 3. `pkg/analytics/escape_code_parser.go` — already parses OSC end-to-end (separate pipeline)

This is the most important finding: **this repo already has a complete,
production byte-level OSC parser**, just wired to a different consumer
(observability/analytics, not status detection).

- `EscapeCodeParser.parseOSC` (`pkg/analytics/escape_code_parser.go:451`)
  correctly scans for both OSC terminators — `BEL` (`0x07`) and `ST`
  (`ESC \`) — and returns a `ParsedEscapeCode{Category: CategoryOSC, RawBytes: ...}`.
- `extractOSCCommand` (`escape_code_parser.go:305`) pulls the numeric OSC
  command prefix (e.g. `"0"`, `"4"`) out of the raw bytes.
- `describeOSC`/`GetOSCDescription` (`escape_code_parser.go:737`) produce a
  human-readable description, and there's a `redactOSCPayloads` flag
  (`escape_code_parser.go:60,264`) — meaning the payload (title text) is
  already being extracted somewhere in this path, just currently discarded
  or hashed for analytics correlation rather than fed anywhere as a status
  signal.
- This parser is wired through `server/server.go`,
  `server/services/escape_code_handler.go`, `server/terminal/escape_scan.go`,
  `session/instance_controller.go`, `server/analytics/escape_event_batch_writer.go`,
  `session/response_stream.go`, and `session/claude_controller.go` — i.e. it
  runs on the PTY stream already, in parallel with (not instead of) the
  detection pipeline in `session/detection`.

**Implication for planning:** there are two live candidate integration points:
(a) extend the narrow `ansiStripRegex` capture in `session/detection/detector.go`
to retain the title payload (lower blast radius, stays inside the detection
package), or (b) tap the existing `EscapeCodeParser` OSC events and route the
title payload into detection as a new consumer. (a) is more self-contained and
matches the acceptance criteria's framing ("captured... during the same pass
that currently strips it"); (b) reuses more code but means threading a second
pipeline's output into `session/detection`, which today has no dependency on
`pkg/analytics` (worth confirming there's no import cycle risk before choosing
this path — `pkg/analytics` imports `pkg/ansi`, and `session/detection` also
imports `pkg/ansi`, but neither currently imports the other).

### 4. `session/detection/detector.go:129` — the exact regex named in requirements

```go
var ansiStripRegex = regexp.MustCompile(`\x1b\[[0-9;]*` + ansi.CSIFinalByteClass + `|\x1b\][^\x07]*\x07|\x1b[()][A-Za-z0-9]`)
```

The middle alternative, `\x1b\][^\x07]*\x07`, is the OSC branch. It already
correctly isolates the OSC payload as a capture opportunity — `[^\x07]*` is
exactly the title/command content between `\x1b]` and the terminating `BEL`.
Today `stripANSI` (`detector.go:141`) just replaces the whole match with `""`
via `ansiStripRegex.ReplaceAllString`, discarding the captured content.

**This regex can be extended to capture rather than only strip** in one of two
ways://
- Add a capturing group to `ansiStripRegex` — `\x1b\](\d+);([^\x07]*)\x07` — and
  run a *separate*, `FindAllSubmatch`-based extraction pass alongside (not
  replacing) the existing `ReplaceAllString` strip call, so stripped-text
  behavior for downstream pattern matching is provably unchanged (acceptance
  criterion 1). This is the lower-risk approach — it doesn't touch the
  strip regex's replace semantics at all, just adds a second read-only pass
  over the same bytes.
- Reuse `pkg/analytics`'s `parseOSC`/`extractOSCCommand` machinery instead of a
  second regex, if the plan favors path (b) above.

One caveat worth flagging for planning: `[^\x07]*` in the strip regex does not
match the `ESC \` (ST) terminator form that Claude Code (and `pkg/analytics`'s
`parseOSC`) also handles for OSC sequences — only BEL-terminated OSC is
captured today. Research didn't confirm whether Claude Code's title OSC ever
uses the ST form in practice (herdr's reference manifest — see below — assumes
BEL), but the plan should note this as a known gap or add the ST alternative
too (mirroring `pkg/analytics/escape_code_parser.go:470`'s BEL-or-ST handling)
for parity with the more complete existing parser.

## Where raw PTY bytes already reach the detector (integration point for priority override)

Confirmed via `session/detection/detector.go` and `pattern_set.go`:

- `StatusDetector.Detect(output []byte) DetectedStatus` (`detector.go:273`)
  receives the **raw, unstripped** PTY bytes as `output`, calls
  `sd.normalizer.Normalize(string(output))` to get stripped `text`, then calls
  `sd.detectFromText(text, output)` — passing **both** the stripped text *and*
  the original raw bytes through to `PatternSet.MatchLines(text, rawPTY)`
  (`pattern_set.go:69`).
- `MatchLines` already special-cases raw-byte-only signals that have no
  stripped-text equivalent — e.g. `hasScreenOverwrite(rawPTY)` at
  `pattern_set.go:125`, which backs `HasActiveScreenRedraw`. This is the
  existing pattern for "a raw-byte signal overrides/supplements text pattern
  matching," and OSC title detection is structurally the same kind of check —
  it slots into the same function, near the same rawPTY special-casing, rather
  than requiring new plumbing from the PTY layer up to `binaries/claude.go`.
- `TerminalDetector` (`terminal_detector.go`), the consumer-side interface,
  already exposes `Detect(output []byte)`, `DetectFromLines(lines []string)`,
  etc. — both byte-based and line-based entry points exist today, so an
  OSC-title field can be threaded through whichever entry point the calling
  code uses without changing the interface's raw-bytes contract at the
  `Detect` call site (only `DetectFromLines`, which only ever received
  pre-stripped `content string`/`[]string`, would need a new parameter or a
  parallel path if OSC support is required there too — worth resolving during
  planning whether OSC-title capture needs to reach both entry points or just
  `Detect`).

## Where the debounce/stabilization delay lives (bypass target for AC #6)

Found in `session/detection/idle.go`, **not** in `detector.go` itself — the
`StatusDetector`/`PatternSet` layer is stateless per call; debouncing is a
property of the stateful `IdleDetector` wrapper:

```go
// idle.go:141-147 (DetectState) and :173-179 (DetectStateFromContent) — identical pattern
newState := id.mapStatusToIdleState(status)
if newState != id.currentState {
    if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= id.config.DebounceDelay {
        id.currentState = newState
        id.lastStateChange = id.timeNow()
    }
}
```

A newly-computed state is only committed if enough wall-clock time
(`id.config.DebounceDelay`) has passed since the last committed transition;
otherwise the computed state is silently dropped and the old `currentState`
persists. This is the mechanism AC #6 asks to bypass for OSC-derived
transitions. Because both `DetectState()` and `DetectStateFromContent()`
already receive the byte/line data needed to compute `status` before this
debounce gate, the natural bypass point is: if `status` carries (or is
accompanied by) an OSC-derived signal, skip the `DebounceDelay` check and
commit immediately — i.e. a boolean/flag threaded alongside `DetectedStatus`
(or a distinct return value indicating "OSC-sourced, high confidence") rather
than a structural rewrite of the debounce mechanism itself. This keeps the
change additive and satisfies the non-goal "changing the debounce architecture
itself" is out of scope — only a bypass path is being added.

## Braille spinner / `✳` character detection — no library needed

U+2800–U+28FF (Braille Patterns block, covers all of ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) and `✳`
(U+2733, EIGHT SPOKED ASTERISK) are both single Unicode code points with no
combining/normalization ambiguity — a simple `unicode.In` range check (Braille)
or direct rune comparison (`✳`) is sufficient; no `golang.org/x/text` or
grapheme-cluster library is needed. The repo already does exactly this kind of
inline rune-class check for other spinner glyphs — see the `claude_thinking_verb`
pattern in `session/detection/binaries/claude.go:127`
(`[·✢✳✶✻✽●*✦]` character class in a compiled regexp) and the
`progress_indicators` pattern at `claude.go:139` (`[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏★]`) — **the
exact Braille spinner glyphs named in the requirements are already present
in an existing regex pattern** in this file, just applied to stripped screen
text rather than to OSC title content. This strongly suggests the simplest
implementation reuses the same character set/regex against the captured OSC
title string, rather than introducing a new Unicode range table.

## Community practice (external validation)

Go's terminal/ANSI ecosystem splits into two camps, neither of which fits this
task:
- **Full terminal emulators** (e.g. `github.com/charmbracelet/x/vt`,
  `hinshun/vt10x`) — implement a virtual terminal screen buffer to answer "what
  does the screen look like now." Overkill for extracting one specific escape
  sequence's payload from a byte stream; would require a new direct dependency
  for a problem this repo already solves twice in-house.
- **Style/strip-only libraries** (e.g. `muesli/reflow`, `acarl005/stripansi`,
  `mgutz/ansi`) — handle SGR color codes for terminal *output* formatting, not
  *input* parsing/extraction of OSC payloads; not built for this direction of
  data flow.

Narrow, single-purpose OSC/CSI extraction (exactly this task's shape) is
routinely hand-rolled via regex or a small byte-scanning state machine in Go
tooling that needs to *read* terminal output rather than *render* it — which
matches this repo's own prior art (`pkg/ansi`, `pkg/analytics/escape_code_parser.go`)
and is consistent with the reference implementation cited in requirements
(herdr, a Rust project, also hand-rolls its OSC/spinner detection rather than
depending on a terminal-emulation crate — `src/pane/agent_detection.rs`).

**Conclusion: hand-rolled parsing (extending the existing regex or reusing
`pkg/analytics`'s byte-scanner) is both the community norm for this narrow a
task and the path of least resistance given what's already in this codebase.**

## Recommendation for planning phase

1. No new Go dependency.
2. Extend `ansiStripRegex`'s OSC alternative (or add a sibling regex) in
   `session/detection/detector.go` to capture the title payload via a
   `FindAllSubmatch`-style read-only pass, run alongside (not replacing) the
   existing `ReplaceAllString` strip — satisfies AC #1 without touching
   stripped-text output.
3. Decide during planning whether to reuse `pkg/analytics`'s more complete
   OSC parser (handles both BEL and ST terminators) vs. a second, narrower
   regex scoped to just title (`]0;`) and progress (`]4;0`) — leaning toward
   the narrower regex for lower blast radius and no new cross-package
   dependency from `session/detection` into `pkg/analytics`, but note the ST
   terminator gap either way.
4. Thread the captured title string as a new parameter/field through
   `detectFromText`/`PatternSet.MatchLines`, alongside the existing `rawPTY
   []byte` parameter — same call shape as the existing `hasScreenOverwrite`
   raw-byte special case.
5. Add spinner/✳ detection in `binaries/claude.go`, reusing the existing
   Braille+asterisk character class already present in the
   `claude_thinking_verb`/`progress_indicators` patterns (just applied to OSC
   title text instead of screen text).
6. For the debounce bypass (AC #6), add a signal (bool or richer type) that
   lets `session/detection/idle.go`'s `DetectState`/`DetectStateFromContent`
   skip the `DebounceDelay` gate when the status came from OSC — additive
   change, not a rewrite of the debounce mechanism.

## Sources

- `go.mod` (repo root) — dependency list, verified no OSC-parsing library present
- `pkg/ansi/csi.go` — existing shared CSI regex/byte-range helper, doc comment on why OSC isn't included
- `pkg/analytics/escape_code_parser.go:1-60,299-320,450-483,736-765` — full existing OSC parser (parseOSC, extractOSCCommand, describeOSC, redactOSCPayloads)
- `session/detection/detector.go:117-278` — `ansiStripRegex`, `stripANSI`, `hasScreenOverwrite`, `Detect`, `detectFromText`
- `session/detection/pattern_set.go:69,125` — `MatchLines` raw-byte special-casing (`hasScreenOverwrite`)
- `session/detection/terminal_detector.go` — `TerminalDetector` interface, raw-bytes vs. lines entry points
- `session/detection/idle.go:100-200` — `IdleDetector.DetectState`/`DetectStateFromContent`, `DebounceDelay` gate
- `session/detection/binaries/claude.go:127,139` — existing Braille spinner + asterisk character classes in `claude_thinking_verb`/`progress_indicators` patterns
- `session/detection/dtypes/dtypes.go` — `StatusPattern`, `StatusPatterns`, `BinaryDetector` interface shapes
- Requirements doc's cited reference: herdr (https://github.com/ogulcancelik/herdr), `src/pane/agent_detection.rs` — hand-rolled OSC/spinner detection precedent in a comparable tool, not backed by a terminal-emulation crate
